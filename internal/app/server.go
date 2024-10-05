package server

import (
	"context"
	"database/sql"
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
	db, err := config.Config.Db.Connect()
	if err != nil {
		log.Fatal("could not connect to db: %w", err)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	store := model.NewStore(db)

	resendClient := resend.NewClient(config.Config.Resend.ApiKey)
	server := &server{
		client:       client,
		store:        store,
		resendClient: resendClient,
	}

	subdomainMiddleware := subdomain.NewSubdomainMiddleware(store)
	unauthMiddleware := auth.NewUnauthMiddleware(store)
	authMiddleware := auth.NewAuthMiddleware(store)
	blogMiddleware := blog.NewBlogMiddleware(store)
	billingService := billing.NewBillingService(store)

	authService := auth.NewAuthService(client, resendClient, store, &config.Config.Github)
	installService := installation.NewInstallationService(client, resendClient, store, &config.Config)
	blogService := blog.NewBlogService(store, resendClient)

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
	authR.HandleFunc("/home", home(server))
	authR.HandleFunc("/home/callback", homecallback(server))
	authR.HandleFunc("/gh/linkgithub", authService.LinkGithubAccount())
	authR.HandleFunc("/stripe/create-checkout-session", billingService.CreateCheckoutSession()).Methods("POST")
	authR.HandleFunc("/stripe/success", billingService.Success())
	authR.HandleFunc("/stripe/cancel", billingService.Cancel())

	blogR := authR.PathPrefix("/blogs/{blogID}").Subrouter()
	blogR.Use(blogMiddleware.AuthoriseBlog)
	blogR.HandleFunc("/config", blogService.Config())
	blogR.HandleFunc("/subdomain/check", blogService.SubdomainCheck())
	blogR.HandleFunc("/subdomain/submit", blogService.SubdomainSubmit())
	blogR.HandleFunc("/generate-demo", blogService.LaunchDemoBlog())
	blogR.HandleFunc("/dashboard", blogService.SubscriberMetrics())

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

func homecallback(s *server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("homecallback handler...")

		/* get session */
		session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if !ok {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}

		start := time.Now()
		for {
			duration := time.Since(start)
			log.Printf("duration: %s\n", duration.String())
			if duration > 10*time.Second {
				log.Printf("homecallback handler timed out for user `%s'\n", session.UserID)
				http.Error(w, "callback timed out", http.StatusRequestTimeout)
				return
			}

			/* get installation info */
			_, err := getInstallationsInfo(s.store, session.UserID)
			if err != nil {
				if err != sql.ErrNoRows {
					log.Printf("error fetching installations info for user `%d': %v\n", session.UserID, err)
					http.Error(w, "", http.StatusInternalServerError)
					return
				}
				/* not found, wait for event to be processed */
				time.Sleep(2 * time.Second)
				continue
			}
			/* got installation info */
			break
		}
		/* found installation info */
		http.Redirect(w, r, "/user/home", http.StatusSeeOther)
	}
}

func home(s *server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("home handler...")
		/* XXX: add metrics */

		/* get session */
		session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if !ok {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}

		installationsInfo, err := getInstallationsInfo(s.store, session.UserID)
		if err != nil {
			log.Printf("could not get Installations info for user `%d': %v", session.UserID, err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		ghInstallUrl := fmt.Sprintf(ghInstallUrlTemplate, config.Config.Github.AppName)
		util.ExecTemplate(w, []string{"home.html", "installations.html", "blogs.html"},
			util.PageInfo{
				Data: struct {
					Title               string
					Session             *auth.Session
					Username            string
					GithubAppInstallUrl string
					Installations       []InstallationInfo
				}{
					Title:               "Home",
					Session:             session,
					Username:            session.Username,
					GithubAppInstallUrl: ghInstallUrl,
					Installations:       installationsInfo,
				},
			},
			template.FuncMap{},
		)
	}
}

type InstallationInfo struct {
	GithubID  int64      `json:"github_id"`
	CreatedAt time.Time  `json:"created_at"`
	Blogs     []BlogInfo `json:"blogs"`
}

type BlogInfo struct {
	ID         int32     `json:"ID"`
	Name       string    `json:"name"`
	GithubUrl  string    `json:"html_url"`
	WebsiteUrl string    `json:"website_url"`
	Subdomain  string    `json:"subdomain"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
}

func getInstallationsInfo(s *model.Store, userID int32) ([]InstallationInfo, error) {
	/* get installations for user */
	installations, err := s.ListInstallationsForUser(context.TODO(), userID)
	if err != nil {
		return []InstallationInfo{}, err
	}
	/* populate the installation info get repositories */
	var info []InstallationInfo
	for _, dbInstallation := range installations {
		blogsInfo, err := getBlogsInfo(s, dbInstallation.GhInstallationID)
		if err != nil {
			return []InstallationInfo{}, fmt.Errorf("error getting RepositoriesInfo: %w", err)
		}
		installationInfo := InstallationInfo{
			GithubID:  dbInstallation.GhInstallationID,
			CreatedAt: dbInstallation.CreatedAt,
			Blogs:     blogsInfo,
		}
		info = append(info, installationInfo)
	}
	return info, nil
}

func getBlogsInfo(s *model.Store, ghInstallationID int64) ([]BlogInfo, error) {
	blogs, err := s.ListBlogsForInstallationByGhInstallationID(context.TODO(), ghInstallationID)
	if err != nil {
		/* should not be possible to have an installation with no repositories */
		return []BlogInfo{}, err
	}
	var info []BlogInfo
	for _, blog := range blogs {
		subdomain := "Not configured"
		if blog.Subdomain.Valid {
			subdomain = blog.Subdomain.String
		}
		blogInfo := BlogInfo{
			ID:         blog.ID,
			Name:       blog.GhName,
			GithubUrl:  blog.GhUrl,
			WebsiteUrl: fmt.Sprintf("http://%s.localhost:7999", blog.DemoSubdomain), /* XXX: how to accurately track this since website is generated from repos */
			Subdomain:  subdomain,
			Active:     blog.Active,
			CreatedAt:  blog.CreatedAt,
		}
		info = append(info, blogInfo)
	}
	return info, nil
}
