package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"text/template"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/app/handler"
	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/authn"
	"github.com/xr0-org/progstack/internal/billing"
	"github.com/xr0-org/progstack/internal/blog"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/installation"
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
	r.Use(metrics.Middleware)
	r.Use(routing.NewRoutingService(store).Middleware)

	/* public routes */

	/* init services */
	billingService := billing.NewBillingService(store)
	blogService := blog.NewBlogService(httpClient, store)

	/* init metrics */
	metrics.Initialize()

	h := handler.NewHandler(store)

	h.Handle(r, "/", index)
	authNService := authn.NewAuthNService(httpClient, store)
	h.Handle(r, "/register", authNService.Register)
	h.Handle(r, "/login", authNService.Login)
	h.Handle(r, "/gh/login", authNService.GithubLogin)
	h.Handle(r, "/gh/oauthcallback", authNService.GithubOAuthCallback)
	h.Handle(r, "/gh/linkcallback", authNService.GithubLinkCallback)
	h.Handle(r, "/magic/register", authNService.MagicRegister)
	h.Handle(r, "/magic/registercallback", authNService.MagicRegisterCallback)
	h.Handle(r, "/magic/login", authNService.MagicLogin)
	h.Handle(r, "/magic/logincallback", authNService.MagicLoginCallback)
	h.Handle(
		r,
		"/gh/installcallback",
		installation.NewInstallationService(
			httpClient, store,
		).InstallationCallback,
	)
	h.Handle(r, "/stripe/webhook", billingService.StripeWebhook)
	h.Handle(r, "/pricing", billingService.Pricing)

	h.Handle(r, "/blogs/{blogID}/subscribe", blogService.SubscribeToBlog).Methods("POST")
	h.Handle(r, "/blogs/unsubscribe", blogService.UnsubscribeFromBlog)

	/* authenticated routes */
	authR := r.PathPrefix("/user").Subrouter()
	authR.Use(authn.Middleware)
	userService := user.NewUserService(store)
	h.Handle(authR, "/", userService.Home)
	h.Handle(authR, "/auth/logout", authNService.Logout)
	h.Handle(authR, "/gh/linkgithub", authNService.LinkGithubAccount)
	h.Handle(authR, "/account", userService.Account)
	h.Handle(authR, "/subdomain-check", blogService.SubdomainCheck)
	h.Handle(authR, "/create-new-blog", userService.CreateNewBlog)
	h.Handle(authR, "/repository-flow", userService.RepositoryFlow)
	h.Handle(authR, "/github-installation", userService.GithubInstallation)
	h.Handle(authR, "/folder-flow", userService.FolderFlow)
	h.Handle(authR, "/create-repository-blog", blogService.CreateRepositoryBlog)
	h.Handle(authR, "/create-folder-blog", blogService.CreateFolderBlog)

	/* billing */
	h.Handle(authR, "/stripe/billing-portal", billingService.BillingPortal)

	blogR := authR.PathPrefix("/blogs/{blogID}").Subrouter()
	blogR.Use(blogService.Middleware)
	h.Handle(blogR, "/config", blogService.Config)
	h.Handle(blogR, "/set-subdomain", blogService.SubdomainSubmit)
	h.Handle(blogR, "/config-domain", blogService.ConfigDomain)
	h.Handle(blogR, "/set-domain", blogService.DomainSubmit)
	h.Handle(blogR, "/set-theme", blogService.ThemeSubmit)
	h.Handle(blogR, "/set-test-branch", blogService.TestBranchSubmit)
	h.Handle(blogR, "/set-live-branch", blogService.LiveBranchSubmit)
	h.Handle(blogR, "/set-folder", blogService.FolderSubmit)
	h.Handle(blogR, "/set-status", blogService.SetStatusSubmit)
	h.Handle(blogR, "/sync", blogService.SyncRepository)
	h.Handle(blogR, "/email", blogService.SendPostEmail)

	h.Handle(blogR, "/metrics", blogService.SiteMetrics)
	h.Handle(blogR, "/subscriber/metrics", blogService.SubscriberMetrics)
	h.Handle(blogR, "/subscriber/export", blogService.ExportSubscribers)
	h.Handle(blogR, "/subscriber/edit", blogService.EditSubscriber)
	h.Handle(blogR, "/subscriber/delete", blogService.DeleteSubscriber)

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
