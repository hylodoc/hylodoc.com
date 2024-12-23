package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"text/template"
	"time"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/analytics"
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
	httpPort      = 80
	httpsPort     = 443
	clientTimeout = 30 * time.Second
)

func Serve(store *model.Store) error {
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
	httpClient := httpclient.NewHttpClient(clientTimeout)
	mixpanelClient := analytics.NewMixpanelClientWrapper(
		config.Config.Mixpanel.Token,
	)
	authNService := authn.NewAuthNService(httpClient, store, mixpanelClient)
	userService := user.NewUserService(store, mixpanelClient)
	billingService := billing.NewBillingService(store, mixpanelClient)
	installationService := installation.NewInstallationService(
		httpClient, store,
	)
	blogService := blog.NewBlogService(httpClient, store, mixpanelClient)

	/* init metrics */
	metrics.Initialize()
	r.Handle("/metrics", metrics.Handler())

	r.HandleFunc("/", index(mixpanelClient))
	r.HandleFunc("/register", authNService.Register())
	r.HandleFunc("/login", authNService.Login())
	r.HandleFunc("/gh/login", authNService.GithubLogin())
	r.HandleFunc("/gh/oauthcallback", authNService.GithubOAuthCallback())
	r.HandleFunc("/gh/linkcallback", authNService.GithubLinkCallback())
	r.HandleFunc("/magic/register", authNService.MagicRegister())
	r.HandleFunc("/magic/registercallback", authNService.MagicRegisterCallback())
	r.HandleFunc("/magic/login", authNService.MagicLogin())
	r.HandleFunc("/magic/logincallback", authNService.MagicLoginCallback())
	r.HandleFunc("/gh/installcallback", installationService.InstallationCallback())
	r.HandleFunc("/stripe/webhook", billingService.StripeWebhook())
	r.HandleFunc("/pricing", billingService.Pricing())

	r.HandleFunc("/blogs/{blogID}/subscribe", blogService.SubscribeToBlog()).Methods("POST")
	r.HandleFunc("/blogs/unsubscribe", blogService.UnsubscribeFromBlog())

	/* authenticated routes */
	authR := r.PathPrefix("/user").Subrouter()
	authR.Use(authn.Middleware)
	authR.HandleFunc("/", userService.Home())
	authR.HandleFunc("/auth/logout", authNService.Logout())
	authR.HandleFunc("/gh/linkgithub", authNService.LinkGithubAccount())
	authR.HandleFunc("/account", userService.Account())
	authR.HandleFunc("/subdomain-check", blogService.SubdomainCheck())
	authR.HandleFunc("/create-new-blog", userService.CreateNewBlog())
	authR.HandleFunc("/repository-flow", userService.RepositoryFlow())
	authR.HandleFunc("/github-installation", userService.GithubInstallation())
	authR.HandleFunc("/folder-flow", userService.FolderFlow())
	authR.HandleFunc("/create-repository-blog", blogService.CreateRepositoryBlog())
	authR.HandleFunc("/create-folder-blog", blogService.CreateFolderBlog())

	/* billing */
	authR.HandleFunc("/stripe/billing-portal", billingService.BillingPortal())

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
	blogR.HandleFunc("/email", blogService.SendPostEmail())

	blogR.HandleFunc("/metrics", blogService.SiteMetrics())
	blogR.HandleFunc("/subscriber/metrics", blogService.SubscriberMetrics())
	blogR.HandleFunc("/subscriber/export", blogService.ExportSubscribers())
	blogR.HandleFunc("/subscriber/edit", blogService.EditSubscriber())
	blogR.HandleFunc("/subscriber/delete", blogService.DeleteSubscriber())

	/* serve static content */
	r.PathPrefix("/static/css").Handler(http.StripPrefix("/static/css", http.FileServer(http.Dir("./web/static/css"))))
	r.PathPrefix("/static/js").Handler(http.StripPrefix("/static/js", http.FileServer(http.Dir("./web/static/js"))))
	r.PathPrefix("/static/img").Handler(http.StripPrefix("/static/img", http.FileServer(http.Dir("./web/static/img"))))

	/* register subrouter */
	r.PathPrefix("/").Handler(authR)

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

func index(mixpanel *analytics.MixpanelClientWrapper) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("Index handler...")

		mixpanel.Track("Index", r)

		/* get email/username from context */
		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No session")
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		if sesh.IsAuthenticated() {
			logger.Println("Redirecting unauthenticated user")
			http.Redirect(w, r, "/user/", http.StatusSeeOther)
			return
		}

		/* get repositories for unauth session */

		util.ExecTemplate(w, []string{"index.html"},
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
		)
	}
}
