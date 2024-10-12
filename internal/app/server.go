package server

import (
	"fmt"
	"log"
	"net/http"
	"text/template"
	"time"

	"github.com/gorilla/mux"
	"github.com/resend/resend-go/v2"
	"github.com/spf13/viper"
	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/billing"
	"github.com/xr0-org/progstack/internal/blog"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/installation"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/subdomain"
	"github.com/xr0-org/progstack/internal/user"
	"github.com/xr0-org/progstack/internal/util"
)

const (
	Cssdir = "web/static/css"

	listeningPort = 7999 /* XXX: make configurable */

	ghInstallUrlTemplate = "https://github.com/apps/%s/installations/new"

	clientTimeout = 3 * time.Second
)

type server struct {
	client       *http.Client
	store        *model.Store
	resendClient *resend.Client
}

func init() {
	viper.SetConfigFile("conf.yaml")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		log.Fatalf("Error reading .env file: %s", err)
	}
	if err := viper.Unmarshal(&config.Config); err != nil {
		log.Fatalf("Error unmarshaling config: %s", err)
	}
	log.Printf("loaded config: %+v\n", config.Config)
}

func Serve() {
	/* init dependencies */
	db, err := config.Config.Db.Connect()
	if err != nil {
		log.Fatal("could not connect to db: %w", err)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	store := model.NewStore(db)
	resendClient := resend.NewClient(config.Config.Resend.ApiKey)

	/* init middleware */
	subdomainMiddleware := subdomain.NewSubdomainMiddleware(store)
	unauthMiddleware := auth.NewUnauthMiddleware(store)
	authMiddleware := auth.NewAuthMiddleware(store)
	blogMiddleware := blog.NewBlogMiddleware(store)
	billingService := billing.NewBillingService(store)

	/* init services */
	authService := auth.NewAuthService(client, resendClient, store, &config.Config.Github)
	userService := user.NewUserService(store)
	installService := installation.NewInstallationService(client, resendClient, store, &config.Config)
	blogService := blog.NewBlogService(store, resendClient)

	/* routes */
	r := mux.NewRouter()

	/* NOTE: userWebsite middleware currently runs before main application */
	r.Use(subdomainMiddleware.RouteToSubdomains)

	r.Use(unauthMiddleware.HandleUnauthSession)

	/* public routes */
	r.HandleFunc("/", index())
	r.HandleFunc("/register", register())
	r.HandleFunc("/login", login())
	r.HandleFunc("/gh/login", authService.GithubLogin())
	r.HandleFunc("/gh/oauthcallback", authService.GithubOAuthCallback())
	r.HandleFunc("/gh/linkcallback", authService.GithubLinkCallback())
	r.HandleFunc("/magic/register", authService.MagicRegister())
	r.HandleFunc("/magic/registercallback", authService.MagicRegisterCallback())
	r.HandleFunc("/magic/login", authService.MagicLogin())
	r.HandleFunc("/magic/logincallback", authService.MagicLoginCallback())
	r.HandleFunc("/gh/installcallback", installService.InstallationCallback())
	r.HandleFunc("/stripe/webhook", billingService.StripeWebhook())

	/* XXX: should operate on subdomain since we route with that, then we can get the associated blog info */
	r.HandleFunc("/blogs/{blogID}/subscribe", blogService.SubscribeToBlog()).Methods("POST")
	r.HandleFunc("/blogs/{blogID}/unsubscribe", blogService.UnsubscribeFromBlog())

	/* authenticated routes */
	authR := r.PathPrefix("/user").Subrouter()
	authR.Use(authMiddleware.ValidateAuthSession)
	authR.HandleFunc("/auth/logout", authService.Logout())
	authR.HandleFunc("/gh/linkgithub", authService.LinkGithubAccount())
	authR.HandleFunc("/account", userService.Account())
	authR.HandleFunc("/delete", userService.Delete())
	authR.HandleFunc("/home", userService.Home())
	authR.HandleFunc("/stripe/subscriptions", billingService.Subscriptions())
	authR.HandleFunc("/stripe/create-checkout-session", billingService.CreateCheckoutSession())
	authR.HandleFunc("/stripe/success", billingService.Success())
	authR.HandleFunc("/stripe/cancel", billingService.Cancel())
	authR.HandleFunc("/stripe/billing-portal", billingService.BillingPortal())

	blogR := authR.PathPrefix("/blogs/{blogID}").Subrouter()
	blogR.Use(blogMiddleware.AuthoriseBlog)
	blogR.HandleFunc("/config", blogService.Config())
	blogR.HandleFunc("/subdomain-check", blogService.SubdomainCheck())
	blogR.HandleFunc("/subdomain-submit", blogService.SubdomainSubmit())
	blogR.HandleFunc("/generate-demo", blogService.LaunchDemoBlog())
	blogR.HandleFunc("/subscriber/metrics", blogService.SubscriberMetrics())
	blogR.HandleFunc("/subscriber/export", blogService.ExportSubscribers())
	blogR.HandleFunc("/set-test-branch", blogService.TestBranchSubmit())
	blogR.HandleFunc("/set-live-branch", blogService.LiveBranchSubmit())
	blogR.HandleFunc("/set-status", blogService.SetStatusSubmit())

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

func index() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("index handler...")
		/* XXX: add metrics */

		/* get email/username from context */
		session, _ := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if session != nil {
			http.Redirect(w, r, "/user/home", http.StatusSeeOther)
		}

		util.ExecTemplate(w, []string{"index.html"},
			util.PageInfo{
				Data: struct {
					Title   string
					Session *auth.Session
				}{
					Title:   "Progstack - blogging for devs",
					Session: session,
				},
			},
			template.FuncMap{},
		)
	}
}

func register() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("register handler...")
		/* XXX: add metrics */

		/* get email/username from context */
		session, _ := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if session != nil {
			http.Redirect(w, r, "/user/home", http.StatusSeeOther)
		}

		util.ExecTemplate(w, []string{"register.html"},
			util.PageInfo{
				Data: struct {
					Title   string
					Session *auth.Session
				}{
					Title:   "Progstack - blogging for devs",
					Session: session,
				},
			},
			template.FuncMap{},
		)
	}
}

func login() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("login handler...")
		/* XXX: add metrics */

		/* get email/username from context */
		session, _ := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if session != nil {
			http.Redirect(w, r, "/user/home", http.StatusSeeOther)
		}

		util.ExecTemplate(w, []string{"login.html"},
			util.PageInfo{
				Data: struct {
					Title   string
					Session *auth.Session
				}{
					Title:   "Progstack - blogging for devs",
					Session: session,
				},
			},
			template.FuncMap{},
		)
	}
}
