package server

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/viper"
	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/util"
)

const (
	Tmpldir = "web/templates"
	Cssdir  = "web/static/css"
	Hugodir = "web/static/hugo/public/"

	listeningPort = 7999

	ghAuthUrl     = "https://github.com/login/oauth/authorize"
	ghTokenUrl    = "https://github.com/login/oauth/access_token"
	ghUserUrl     = "https://api.github.com/user"
	ghInstallUrl  = "https://github.com/apps/%s/installations/new"
	clientTimeout = 3 * time.Second
)

type server struct {
	client *http.Client
	store  *model.Store
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

	server := &server{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		store: model.NewStore(db),
	}

	unauthMiddleware := auth.NewUnauthMiddleware(server.store)
	authMiddleware := auth.NewAuthMiddleware(server.store)

	r := mux.NewRouter()
	r.Use(unauthMiddleware.HandleUnauthSession)

	/* public routes */
	r.HandleFunc("/", index())
	r.HandleFunc("/login", login())
	r.HandleFunc("/auth/github/callback", authcallback(server))

	/* authenticated routes */
	authR := mux.NewRouter()
	authR.Use(authMiddleware.ValidateAuthSession)
	authR.HandleFunc("/home", home(server))
	authR.HandleFunc("/events", events())
	authR.HandleFunc("/logout", logout(server))

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
		/* XXX: add metrics */

		/* get email/username from context */
		user, _ := r.Context().Value("user").(*auth.User)

		execTemplate(w, []string{"index.html"},
			PageInfo{
				Data: struct {
					Title string
					User  *auth.User
				}{
					Title: "Progstack - blogging for devs",
					User:  user,
				},
			},
		)
	}
}

func login() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		/* XXX: add metrics */

		authUrl, err := buildAuthUrl()
		if err != nil {
			log.Println("error in login", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		/* redirect user to GitHub for OAuth authorization */
		http.Redirect(w, r, authUrl, http.StatusFound)
	}
}

func buildAuthUrl() (string, error) {
	u, err := url.Parse(ghAuthUrl)
	if err != nil {
		return "", err
	}
	params := url.Values{}
	params.Add("client_id", config.Config.Github.ClientID)
	u.RawQuery = params.Encode()
	return u.String(), nil
}

func logout(s *server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		/* XXX: add metrics */

		user, ok := r.Context().Value("user").(*auth.User)
		if !ok {
			log.Println("error getting user from context")
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}
		err := auth.Logout(user, w, r, s.store)
		if err != nil {
			log.Println("error logging out user: ", err)
			http.Error(w, "", http.StatusInternalServerError)
		}

		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
}

func authcallback(s *server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		/* XXX: add metrics */

		err := authcallbackProcess(w, r, s)
		if err != nil {
			log.Println("error in authcallback", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/home", http.StatusTemporaryRedirect)
	}
}

func authcallbackProcess(w http.ResponseWriter, r *http.Request, s *server) error {
	/* get code */
	queryParams := r.URL.Query()
	code := queryParams.Get("code")

	/* build access token request */
	req, err := util.NewRequestBuilder("POST", ghTokenUrl).
		WithHeader("Content-Type", "application/x-www-form-urlencoded").
		WithFormParam("client_id", config.Config.Github.ClientID).
		WithFormParam("client_secret", config.Config.Github.ClientSecret).
		WithFormParam("code", code).
		Build()
	if err != nil {
		log.Println("error building auth request: ", err)
		return err
	}

	/* get access token */
	resp, err := s.client.Do(req)
	if err != nil {
		log.Println("error sending auth request: ", err)
		return err
	}
	defer resp.Body.Close()

	/* unpack */
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println("error unpacking body: ", err)
		return err
	}
	parsed, err := url.ParseQuery(string(body))
	if err != nil {
		log.Println("error parsing URL-encoded response: ", err)
		return err
	}
	accessToken, err := util.GetQueryParam(parsed, "access_token")
	if err != nil {
		return err
	}
	expiresIn, err := util.GetQueryParam(parsed, "expires_in")
	if err != nil {
		return err
	}
	refreshToken, err := util.GetQueryParam(parsed, "refresh_token")
	if err != nil {
		return err
	}
	scope, err := util.GetQueryParam(parsed, "scope")
	if err != nil {
		return err
	}
	log.Println("access_token: ", accessToken)
	log.Println("expires_in: ", expiresIn)
	log.Println("refresh_token: ", refreshToken)
	log.Println("scope: ", scope)

	/* build user info request */
	req, err = util.NewRequestBuilder("GET", ghUserUrl).
		WithHeader("Authorization", fmt.Sprintf("Bearer %s", accessToken)).
		Build()
	if err != nil {
		log.Println("error building user info request: ", err)
		return err
	}
	/* get user info */
	resp, err = s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Println("error reading response body: ", err)
		return fmt.Errorf("error reading response body: %w", err)
	}
	var user auth.User
	err = json.Unmarshal(body, &user)
	if err != nil {
		log.Println("error unmarshalling JSON: ", err)
		return fmt.Errorf("error unmarshalling JSON: %w", err)
	}
	err = auth.HandleOAuth(&user, w, s.store)
	if err != nil {
		log.Println("error handling OAuth: ", err)
		return fmt.Errorf("error handling OAuth: %w", err)
	}
	return nil
}

func home(s *server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		/* XXX: add metrics */

		user, ok := r.Context().Value("user").(*auth.User)
		if !ok {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}

		execTemplate(w, []string{"home.html"},
			PageInfo{
				Data: struct {
					Title    string
					User     *auth.User
					Username string
				}{
					Title:    "Home",
					User:     user,
					Username: user.Username,
				},
			},
		)
	}
}

func events() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		/* XXX: add metrics */

	}
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
