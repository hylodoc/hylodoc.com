package server

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path"
	"time"

	"github.com/gorilla/mux"
	"github.com/resend/resend-go/v2"
	"github.com/spf13/viper"
	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/blog"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/installation"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/sites"
)

const (
	Tmpldir = "web/templates"
	Cssdir  = "web/static/css"

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

	userWebsiteMiddleware := sites.NewUserWebsiteMiddleware(store)
	unauthMiddleware := auth.NewUnauthMiddleware(store)
	authMiddleware := auth.NewAuthMiddleware(store)

	authService := auth.NewAuthService(client, resendClient, store, &config.Config.Github)
	installService := installation.NewInstallationService(client, resendClient, store, &config.Config)
	blogService := blog.NewBlogService(store, resendClient)

	r := mux.NewRouter()

	/* NOTE: userWebsite middleware runs before main application */
	r.Use(userWebsiteMiddleware.RouteToSubdomains)

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

	r.HandleFunc("/blogs/{blogID}/subscribe", blogService.SubscribeToBlog()).Methods("POST")
	r.HandleFunc("/blogs/{blogID}/unsubscribe", blogService.UnsubscribeFromBlog())

	/* authenticated routes */
	authR := mux.NewRouter()
	authR.Use(authMiddleware.ValidateAuthSession)
	authR.HandleFunc("/home", home(server))
	authR.HandleFunc("/gh/linkgithub", authService.LinkGithubAccount())
	authR.HandleFunc("/auth/logout", authService.Logout())

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
			http.Redirect(w, r, "/home", http.StatusSeeOther)
		}

		execTemplate(w, []string{"index.html"},
			PageInfo{
				Data: struct {
					Title   string
					Session *auth.Session
				}{
					Title:   "Progstack - blogging for devs",
					Session: session,
				},
			},
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
			http.Redirect(w, r, "/home", http.StatusSeeOther)
		}

		execTemplate(w, []string{"register.html"},
			PageInfo{
				Data: struct {
					Title   string
					Session *auth.Session
				}{
					Title:   "Progstack - blogging for devs",
					Session: session,
				},
			},
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
			http.Redirect(w, r, "/home", http.StatusSeeOther)
		}

		execTemplate(w, []string{"login.html"},
			PageInfo{
				Data: struct {
					Title   string
					Session *auth.Session
				}{
					Title:   "Progstack - blogging for devs",
					Session: session,
				},
			},
		)
	}
}

func home(s *server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("home handler...")
		/* XXX: add metrics */

		/* Get session */
		session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if !ok {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}

		installationInfo, err := getInstallationsInfo(s.store, session.UserID)
		if err != nil {
			log.Printf("error getting installations info: %w", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		ghInstallUrl := fmt.Sprintf(ghInstallUrlTemplate, config.Config.Github.AppName)
		execTemplate(w, []string{"home.html", "repos.html"},
			PageInfo{
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
					Installations:       installationInfo,
				},
			},
		)
	}
}

type InstallationInfo struct {
	GithubID  int64
	CreatedAt time.Time
	Blogs     []BlogInfo
}

type BlogInfo struct {
	Name    string
	HtmlUrl string
}

func getInstallationsInfo(s *model.Store, userID int32) ([]InstallationInfo, error) {
	/* get installations for user */
	installations, err := s.ListInstallationsForUser(context.TODO(), userID)
	if err != nil {
		if err != sql.ErrNoRows {
			return []InstallationInfo{}, err
		}
		/* no installations, no error */
		return []InstallationInfo{}, nil
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
	blogs, err := s.ListBlogsForInstallationWithGhInstallationID(context.TODO(), ghInstallationID)
	if err != nil {
		/* should not be possible to have an installation with no repositories */
		return []BlogInfo{}, err
	}
	var info []BlogInfo
	for _, blog := range blogs {
		blogInfo := BlogInfo{
			Name:    blog.GhName,
			HtmlUrl: blog.GhUrl,
		}
		info = append(info, blogInfo)
	}
	return info, nil
}

/* execTemplate */

func prependDir(names []string, dir string) []string {
	joined := make([]string, len(names))
	for i := range names {
		joined[i] = path.Join(Tmpldir, dir, names[i])
	}
	return joined
}

/* present on every page */
var pageTemplates []string = []string{
	"base.html", "navbar.html",
}

type PageInfo struct {
	Data       interface{}
	NewUpdates bool
}

func execTemplate(w http.ResponseWriter, names []string, info PageInfo) {
	tmpl, err := template.New(names[0]).ParseFiles(
		append(
			prependDir(names, "pages"),
			prependDir(pageTemplates, "partials")...,
		)...,
	)
	if err != nil {
		log.Println("cannot load template", err)
		http.Error(w, "error loading page", http.StatusInternalServerError)
	}
	if err := tmpl.Execute(w, info); err != nil {
		log.Println("cannot execute template", err)
		http.Error(w, "error loading page", http.StatusInternalServerError)
	}
}
