package server

import (
	"fmt"
	"log"
	"net/http"
	"text/template"
	"time"

	"github.com/gorilla/mux"
	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack/internal/analytics"
	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/billing"
	"github.com/xr0-org/progstack/internal/blog"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/installation"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/metrics"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/subdomain"
	"github.com/xr0-org/progstack/internal/user"
	"github.com/xr0-org/progstack/internal/util"
)

const (
	listeningPort = 7999 /* XXX: make configurable */
	clientTimeout = 30 * time.Second
)

func init() {
	if err := config.LoadConfig("conf.yaml"); err != nil {
		log.Fatalf("failed to laod config: %v", err)
	}
}

func Serve() {
	/* init dependencies */
	db, err := config.Config.Db.Connect()
	if err != nil {
		log.Fatal("could not connect to db: %w", err)
	}
	httpClient := httpclient.NewHttpClient(clientTimeout)
	mixpanelClient := analytics.NewMixpanelClientWrapper(
		config.Config.Mixpanel.Token,
	)
	store := model.NewStore(db)
	resendClient := resend.NewClient(config.Config.Resend.ApiKey)

	/* init services */
	sessionService := session.NewSessionService(store)
	subdomainService := subdomain.NewSubdomainService(store)
	authService := auth.NewAuthService(
		httpClient, resendClient, store, mixpanelClient,
	)
	userService := user.NewUserService(store, mixpanelClient)
	billingService := billing.NewBillingService(store, mixpanelClient)
	installationService := installation.NewInstallationService(
		httpClient, resendClient, store,
	)
	blogService := blog.NewBlogService(
		httpClient, store, resendClient, mixpanelClient,
	)

	/* init metrics */
	metrics.Initialize()

	/* routes */
	r := mux.NewRouter()

	r.Use(sessionService.Middleware)
	r.Use(logging.Middleware)
	r.Use(subdomainService.Middleware)
	r.Use(metrics.Middleware)

	/* public routes */
	r.Handle("/metrics", metrics.Handler())

	r.HandleFunc("/", index(mixpanelClient))
	r.HandleFunc("/register", authService.Register())
	r.HandleFunc("/login", authService.Login())
	r.HandleFunc("/gh/login", authService.GithubLogin())
	r.HandleFunc("/gh/oauthcallback", authService.GithubOAuthCallback())
	r.HandleFunc("/gh/linkcallback", authService.GithubLinkCallback())
	r.HandleFunc("/magic/register", authService.MagicRegister())
	r.HandleFunc("/magic/registercallback", authService.MagicRegisterCallback())
	r.HandleFunc("/magic/login", authService.MagicLogin())
	r.HandleFunc("/magic/logincallback", authService.MagicLoginCallback())
	r.HandleFunc("/gh/installcallback", installationService.InstallationCallback())
	r.HandleFunc("/stripe/webhook", billingService.StripeWebhook())

	r.HandleFunc("/blogs/{blogID}/subscribe", blogService.SubscribeToBlog()).Methods("POST")
	r.HandleFunc("/blogs/{blogID}/unsubscribe", blogService.UnsubscribeFromBlog())

	/* authenticated routes */
	authR := r.PathPrefix("/user").Subrouter()
	authR.Use(auth.AuthMiddleware)
	authR.HandleFunc("/", userService.Home())
	authR.HandleFunc("/auth/logout", authService.Logout())
	authR.HandleFunc("/gh/linkgithub", authService.LinkGithubAccount())
	authR.HandleFunc("/account", userService.Account())
	authR.HandleFunc("/delete", userService.Delete())
	authR.HandleFunc("/subdomain-check", blogService.SubdomainCheck())
	authR.HandleFunc("/create-new-blog", userService.CreateNewBlog())
	authR.HandleFunc("/repository-flow", userService.RepositoryFlow())
	authR.HandleFunc("/github-installation", userService.GithubInstallation())
	authR.HandleFunc("/folder-flow", userService.FolderFlow())
	authR.HandleFunc("/create-repository-blog", blogService.CreateRepositoryBlog())
	authR.HandleFunc("/create-folder-blog", blogService.CreateFolderBlog())
	authR.HandleFunc("/stripe/subscriptions", billingService.Subscriptions())
	authR.HandleFunc("/stripe/create-checkout-session", billingService.CreateCheckoutSession())
	authR.HandleFunc("/stripe/success", billingService.Success())
	authR.HandleFunc("/stripe/cancel", billingService.Cancel())
	authR.HandleFunc("/stripe/billing-portal", billingService.BillingPortal())

	blogR := authR.PathPrefix("/blogs/{blogID}").Subrouter()
	blogR.Use(blogService.Middleware)
	blogR.HandleFunc("/config", blogService.Config())
	blogR.HandleFunc("/set-subdomain", blogService.SubdomainSubmit())
	blogR.HandleFunc("/set-theme", blogService.ThemeSubmit())
	blogR.HandleFunc("/set-test-branch", blogService.TestBranchSubmit())
	blogR.HandleFunc("/set-live-branch", blogService.LiveBranchSubmit())
	blogR.HandleFunc("/set-folder", blogService.FolderSubmit())
	blogR.HandleFunc("/set-status", blogService.SetStatusSubmit())

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

	/* start server on listening port */
	log.Printf("listening at http://localhost:%d...\n", listeningPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", listeningPort), r))
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
