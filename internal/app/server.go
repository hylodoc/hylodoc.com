package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"text/template"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/analytics"
	"github.com/xr0-org/progstack/internal/app/handler"
	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/authn"
	"github.com/xr0-org/progstack/internal/billing"
	"github.com/xr0-org/progstack/internal/blog"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/installation"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/metrics"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/routing"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/user"
	"github.com/xr0-org/progstack/internal/util"
	"golang.org/x/crypto/acme/autocert"
)

const (
	/* TODO: make configurable */
	httpPort        = 80
	httpsPort       = 443
	metricsHttpPort = 8000
)

func Serve(httpClient *httpclient.Client, store *model.Store) error {
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", metrics.Handler())
		log.Printf(
			"listening (metrics) on http://localhost:%d...\n",
			metricsHttpPort,
		)
		if err := http.ListenAndServe(
			fmt.Sprintf(":%d", metricsHttpPort), mux,
		); err != nil {
			log.Fatal("fatal metrics error", err)
		}
	}()

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc(
			"/",
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "")
				http.Redirect(
					w, r,
					fmt.Sprintf(
						"https://%s%s",
						r.Host,
						r.URL.String(),
					),
					http.StatusPermanentRedirect,
				)
			},
		)
		log.Printf(
			"listening (to redirect) on http://localhost:%d...\n",
			httpPort,
		)
		if err := http.ListenAndServe(
			fmt.Sprintf(":%d", httpPort), mux,
		); err != nil {
			log.Fatal("fatal http error", err)
		}
	}()

	bootid, err := store.Boot(context.TODO())
	if err != nil {
		return fmt.Errorf("cannot boot: %w", err)
	}
	log.Println("bootid", bootid)

	r := mux.NewRouter()

	/* middleware */
	r.Use(session.NewSessionService(store).Middleware)
	r.Use(logging.Middleware)
	r.Use(metrics.Middleware)
	r.Use(routing.NewRoutingService(store).Middleware)

	/* public routes */

	/* init services */
	mixpanelClient := analytics.NewMixpanelClientWrapper(
		config.Config.Mixpanel.Token,
	)
	billingService := billing.NewBillingService(store, mixpanelClient)
	blogService := blog.NewBlogService(httpClient, store, mixpanelClient)

	/* init metrics */
	metrics.Initialize()

	r.HandleFunc("/", handler.AsHttp(index))
	authNService := authn.NewAuthNService(httpClient, store, mixpanelClient)
	r.HandleFunc("/register", handler.AsHttp(authNService.Register))
	r.HandleFunc("/login", handler.AsHttp(authNService.Login))
	r.HandleFunc("/gh/login", handler.AsHttp(authNService.GithubLogin))
	r.HandleFunc("/gh/oauthcallback", handler.AsHttp(authNService.GithubOAuthCallback))
	r.HandleFunc("/gh/linkcallback", handler.AsHttp(authNService.GithubLinkCallback))
	r.HandleFunc("/magic/register", handler.AsHttp(authNService.MagicRegister))
	r.HandleFunc("/magic/registercallback", handler.AsHttp(authNService.MagicRegisterCallback))
	r.HandleFunc("/magic/login", handler.AsHttp(authNService.MagicLogin))
	r.HandleFunc("/magic/logincallback", handler.AsHttp(authNService.MagicLoginCallback))
	r.HandleFunc(
		"/gh/installcallback",
		installation.NewInstallationService(
			httpClient, store,
		).InstallationCallback(),
	)
	r.HandleFunc("/stripe/webhook", billingService.StripeWebhook())
	r.HandleFunc("/pricing", handler.AsHttp(billingService.Pricing))

	r.HandleFunc("/blogs/{blogID}/subscribe", blogService.SubscribeToBlog()).Methods("POST")
	r.HandleFunc("/blogs/unsubscribe", blogService.UnsubscribeFromBlog())

	/* authenticated routes */
	authR := r.PathPrefix("/user").Subrouter()
	authR.Use(authn.Middleware)
	userService := user.NewUserService(store, mixpanelClient)
	authR.HandleFunc("/", userService.Home())
	authR.HandleFunc("/auth/logout", handler.AsHttp(authNService.Logout))
	authR.HandleFunc("/gh/linkgithub", handler.AsHttp(authNService.LinkGithubAccount))
	authR.HandleFunc("/account", userService.Account())
	authR.HandleFunc("/subdomain-check", blogService.SubdomainCheck())
	authR.HandleFunc("/create-new-blog", userService.CreateNewBlog())
	authR.HandleFunc("/repository-flow", userService.RepositoryFlow())
	authR.HandleFunc("/github-installation", userService.GithubInstallation())
	authR.HandleFunc("/folder-flow", userService.FolderFlow())
	authR.HandleFunc("/create-repository-blog", blogService.CreateRepositoryBlog())
	authR.HandleFunc("/create-folder-blog", blogService.CreateFolderBlog())

	/* billing */
	authR.HandleFunc("/stripe/billing-portal", handler.AsHttp(billingService.BillingPortal))

	blogR := authR.PathPrefix("/blogs/{blogID}").Subrouter()
	blogR.Use(blogService.Middleware)
	blogR.HandleFunc("/config", blogService.Config())
	blogR.HandleFunc("/set-subdomain", blogService.SubdomainSubmit())
	blogR.HandleFunc("/config-domain", blogService.ConfigDomain())
	blogR.HandleFunc("/set-domain", blogService.DomainSubmit())
	blogR.HandleFunc("/set-theme", blogService.ThemeSubmit())
	blogR.HandleFunc("/set-test-branch", blogService.TestBranchSubmit())
	blogR.HandleFunc("/set-live-branch", blogService.LiveBranchSubmit())
	blogR.HandleFunc("/set-folder", blogService.FolderSubmit())
	blogR.HandleFunc("/set-status", blogService.SetStatusSubmit())
	blogR.HandleFunc("/sync", blogService.SyncRepository())
	blogR.HandleFunc("/email", blogService.SendPostEmail())

	blogR.HandleFunc("/metrics", blogService.SiteMetrics())
	blogR.HandleFunc("/subscriber/metrics", blogService.SubscriberMetrics())
	blogR.HandleFunc("/subscriber/export", blogService.ExportSubscribers())
	blogR.HandleFunc("/subscriber/edit", blogService.EditSubscriber())
	blogR.HandleFunc("/subscriber/delete", blogService.DeleteSubscriber())

	/* serve static content */
	r.PathPrefix("/static/css").Handler(
		http.StripPrefix(
			"/static/css",
			http.FileServer(http.Dir("./web/static/css")),
		),
	)
	r.PathPrefix("/static/js").Handler(
		http.StripPrefix(
			"/static/js",
			http.FileServer(http.Dir("./web/static/js")),
		),
	)
	r.PathPrefix("/static/img").Handler(
		http.StripPrefix(
			"/static/img",
			http.FileServer(http.Dir("./web/static/img")),
		),
	)

	/* XXX: makes `/user` serve `/user/` */
	r.PathPrefix("/").Handler(authR)

	m := &autocert.Manager{
		Cache:  autocert.DirCache(config.Config.Progstack.CertsPath),
		Prompt: autocert.AcceptTOS,
		Email:  "tls@hylo.lbnz.dev",
		HostPolicy: func(ctx context.Context, host string) error {
			return nil
		},
	}
	s := &http.Server{
		Addr:      fmt.Sprintf(":%d", httpsPort),
		TLSConfig: m.TLSConfig(),
		Handler:   r,
	}
	switch config.Config.Progstack.Protocol {
	case "https":
		log.Printf("listening at https://localhost:%d...\n", httpsPort)
		return s.ListenAndServeTLS("", "")
	case "http":
		log.Printf("listening at http://localhost:%d...\n", httpsPort)
		return s.ListenAndServe()
	default:
		return fmt.Errorf("invalid protocol")
	}
}

func index(r request.Request) (response.Response, error) {
	logger := r.Logger()
	logger.Println("Index handler...")

	r.MixpanelTrack("Index")

	sesh := r.Session()

	if sesh.IsAuthenticated() {
		logger.Println("Redirecting unauthenticated user")
		return response.NewRedirect(
			"/user/", http.StatusFound,
		), nil
	}

	return response.NewTemplate(
		[]string{"index.html"},
		util.PageInfo{
			Data: struct {
				Title    string
				UserInfo *session.UserInfo
			}{
				Title:    "Progstack - blogging for devs",
				UserInfo: session.ConvertSessionToUserInfo(sesh),
			},
		},
		template.FuncMap{},
		logger,
	), nil
}
