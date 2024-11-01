package server

import (
	"fmt"
	"log"
	"net/http"
	"text/template"
	"time"

	"github.com/gorilla/mux"
	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/billing"
	"github.com/xr0-org/progstack/internal/blog"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/installation"
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
	client := httpclient.NewHttpClient(clientTimeout)
	store := model.NewStore(db)
	resendClient := resend.NewClient(config.Config.Resend.ApiKey)

	/* init middleware */
	subdomainMiddleware := subdomain.NewSubdomainMiddleware(store)
	sessionMiddleware := session.NewSessionMiddleware(store)
	blogMiddleware := blog.NewBlogMiddleware(store)
	billingService := billing.NewBillingService(store)

	/* init services */
	authService := auth.NewAuthService(client, resendClient, store, &config.Config.Github)
	userService := user.NewUserService(store)
	installService := installation.NewInstallationService(client, resendClient, store, &config.Config)
	blogService := blog.NewBlogService(client, store, resendClient)

	/* init metrics */
	metrics.Initialize()

	/* routes */
	r := mux.NewRouter()

	/* NOTE: userWebsite middleware currently runs before main application */
	r.Use(subdomainMiddleware.RouteToSubdomains)

	r.Use(metrics.MetricsMiddleware)

	r.Use(sessionMiddleware.SessionMiddleware)

	/* public routes */
	r.HandleFunc("/", index())
	r.Handle("/metrics", metrics.Handler())
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
	blogR.Use(blogMiddleware.AuthoriseBlog)
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

func index() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("index handler...")
		/* XXX: add metrics */

		/* get email/username from context */
		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			log.Printf("session not found")
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		if sesh.IsAuthenticated() {
			log.Printf("user authenticated redirecting home...")
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
		)
	}
}

func register() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("register handler...")
		/* XXX: add metrics */

		/* get email/username from context */
		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			http.Redirect(w, r, "/user/", http.StatusSeeOther)
			return
		}

		util.ExecTemplate(w, []string{"register.html"},
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
		)
	}
}

func login() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("login handler...")
		/* XXX: add metrics */

		/* get email/username from context */
		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			http.Redirect(w, r, "/user/", http.StatusSeeOther)
			return
		}

		util.ExecTemplate(w, []string{"login.html"},
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
		)
	}
}
