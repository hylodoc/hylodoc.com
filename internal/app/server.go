package server

import (
	"context"
	"fmt"
	"log"
	"net/http"

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
	r.Use(session.NewSessionService(store).Middleware)
	r.Use(metrics.Middleware)
	r.Use(routing.NewRoutingService(store).Middleware)

	/* MethodNotAllowed handler ignored for now */
	notfoundR := mux.NewRouter()
	notfoundR.Use(session.NewSessionService(store).Middleware)
	notfoundR.Use(metrics.Middleware)
	notfoundR.Use(routing.NewRoutingService(store).Middleware)
	notfoundR.PathPrefix("/").HandlerFunc(handler.NotFound)
	r.NotFoundHandler = notfoundR

	/* public routes */

	/* init services */
	billingService := billing.NewBillingService(store)
	blogService := blog.NewBlogService(httpClient, store)

	/* init metrics */
	metrics.Initialize()

	handler.Handle(r, "/", index)

	authNService := authn.NewAuthNService(httpClient, store)
	handler.Handle(r, "/register", authNService.Register)
	handler.Handle(r, "/login", authNService.Login)
	handler.Handle(r, "/gh/login", authNService.GithubLogin)
	handler.Handle(r, "/gh/oauthcallback", authNService.GithubOAuthCallback)
	handler.Handle(r, "/gh/linkcallback", authNService.GithubLinkCallback)
	handler.Handle(r, "/magic/register", authNService.MagicRegister)
	handler.Handle(r, "/magic/registercallback", authNService.MagicRegisterCallback)
	handler.Handle(r, "/magic/login", authNService.MagicLogin)
	handler.Handle(r, "/magic/logincallback", authNService.MagicLoginCallback)
	handler.Handle(
		r,
		"/gh/installcallback",
		installation.NewInstallationService(
			httpClient, store,
		).InstallationCallback,
	)
	handler.Handle(r, "/stripe/webhook", billingService.StripeWebhook)
	handler.Handle(r, "/pricing", billingService.Pricing)

	handler.Handle(r, "/blogs/{blogID}/subscribe", blogService.SubscribeToBlog).Methods("POST")
	handler.Handle(r, "/blogs/unsubscribe", blogService.UnsubscribeFromBlog)

	/* authenticated routes */
	authR := r.PathPrefix("/user").Subrouter()
	authR.Use(authn.Middleware)
	userService := user.NewUserService(store)
	handler.Handle(authR, "/", userService.Home)
	handler.Handle(authR, "/auth/logout", authNService.Logout)
	handler.Handle(authR, "/gh/linkgithub", authNService.LinkGithubAccount)
	handler.Handle(authR, "/account", userService.Account)
	handler.Handle(authR, "/subdomain-check", blogService.SubdomainCheck)
	handler.Handle(authR, "/create-new-blog", userService.RepositoryFlow)
	handler.Handle(authR, "/github-installation", userService.GithubInstallation)
	handler.Handle(authR, "/create-repository-blog", blogService.CreateRepositoryBlog)

	/* billing */
	handler.Handle(authR, "/stripe/billing-portal", billingService.BillingPortal)

	blogR := authR.PathPrefix("/blogs/{blogID}").Subrouter()
	blogR.Use(blogService.Middleware)
	handler.Handle(blogR, "/config", blogService.Config)
	handler.Handle(blogR, "/set-subdomain", blogService.SubdomainSubmit)
	handler.Handle(blogR, "/config-domain", blogService.ConfigDomain)
	handler.Handle(blogR, "/set-domain", blogService.DomainSubmit)
	handler.Handle(blogR, "/set-theme", blogService.ThemeSubmit)
	handler.Handle(blogR, "/set-live-branch", blogService.LiveBranchSubmit)
	handler.Handle(blogR, "/set-status", blogService.SetStatusSubmit)
	handler.Handle(blogR, "/sync", blogService.SyncRepository)
	handler.Handle(blogR, "/email", blogService.SendPostEmail)

	handler.Handle(blogR, "/metrics", blogService.SiteMetrics)
	handler.Handle(blogR, "/subscriber/metrics", blogService.SubscriberMetrics)
	handler.Handle(blogR, "/subscriber/export", blogService.ExportSubscribers)
	handler.Handle(blogR, "/subscriber/edit", blogService.EditSubscriber)
	handler.Handle(blogR, "/subscriber/delete", blogService.DeleteSubscriber)

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
	sesh := r.Session()
	sesh.Println("Index handler...")

	r.MixpanelTrack("Index")

	if sesh.IsAuthenticated() {
		sesh.Println("Redirecting unauthenticated user")
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
	), nil
}
