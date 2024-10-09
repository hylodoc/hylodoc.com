package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/email"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/util"
)

const (
	authCookieName = "auth_session_id"

	ghAuthUrl  = "https://github.com/login/oauth/authorize"
	ghTokenUrl = "https://github.com/login/oauth/access_token"
	ghUserUrl  = "https://api.github.com/user"

	ghInstallationAccessTokenUrlTemplate = "https://api.github.com/app/installations/%d/access_tokens"

	CtxSessionKey = "session"
)

type AuthService struct {
	store        *model.Store
	client       *http.Client
	resendClient *resend.Client
	config       *config.GithubParams
}

func NewAuthService(c *http.Client, resendClient *resend.Client, s *model.Store, config *config.GithubParams) AuthService {
	return AuthService{
		client:       c,
		resendClient: resendClient,
		store:        s,
		config:       config,
	}
}

/* Github Auth */

func (o *AuthService) GithubLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUrl, err := buildGithubOAuthUrl()
		if err != nil {
			log.Printf("error in login: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		log.Printf("authUrl: %s\n", authUrl)
		/* redirect user to GitHub for OAuth authorization */
		http.Redirect(w, r, authUrl, http.StatusFound)
	}
}

func buildGithubOAuthUrl() (string, error) {
	u, err := url.Parse(ghAuthUrl)
	if err != nil {
		return "", err
	}
	params := url.Values{}
	params.Add("client_id", config.Config.Github.ClientID)
	params.Add("redirect_uri", config.Config.Github.OAuthCallback)
	u.RawQuery = params.Encode()
	return u.String(), nil
}

func (o *AuthService) GithubOAuthCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		/* create user or login user */
		err := o.githubOAuthCallback(w, r)
		if err != nil {
			log.Printf("error in OAuthCallback: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/user/home", http.StatusTemporaryRedirect)
	}
}

func (o *AuthService) githubOAuthCallback(w http.ResponseWriter, r *http.Request) error {
	/* get code */
	queryParams := r.URL.Query()
	code := queryParams.Get("code")

	/* get accessToken */
	accessToken, err := getOauthAccessToken(o.client, code, o.config.ClientID, o.config.ClientSecret)
	if err != nil {
		return err
	}
	/* get user info using token */
	ghUser, err := getGithubUserInfo(o.client, accessToken)
	if err != nil {
		return err
	}

	u, err := o.store.GetGithubAccountByGhUserID(context.TODO(), ghUser.ID)
	userID := u.UserID
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("error checking for user existence: %w", err)
		}
		/* new user signing in with github, create with github account */
		log.Println("creating user and linking to github account in db...")
		err = o.store.CreateUserWithGithubAccountTx(context.TODO(), model.CreateGithubAccountParams{
			GhUserID:   ghUser.ID,
			GhEmail:    ghUser.Email,
			GhUsername: ghUser.Username,
		})
		if err != nil {
			log.Printf("error creating user in db: %v", err)
			return fmt.Errorf("error creating user in db: %w", err)
		}
		/* fetch newly created userID, needed to create Auth session */
		u, err := o.store.GetGithubAccountByGhUserID(context.TODO(), ghUser.ID)
		if err != nil {
			return fmt.Errorf("error fetching")
		}
		userID = u.UserID
	}

	log.Println("got user: ", u)

	/* create Auth Session */
	err = createAuthSession(userID, w, o.store)
	if err != nil {
		log.Printf("error creating auth session: %v", err)
		return fmt.Errorf("error creating auth session: %w", err)
	}
	return nil
}

func getOauthAccessToken(c *http.Client, code, clientID, clientSecret string) (string, error) {
	/* build access token request */
	req, err := util.NewRequestBuilder("POST", ghTokenUrl).
		WithHeader("Content-Type", "application/x-www-form-urlencoded").
		WithFormParam("client_id", config.Config.Github.ClientID).
		WithFormParam("client_secret", config.Config.Github.ClientSecret).
		WithFormParam("code", code).
		Build()
	if err != nil {
		log.Printf("error building auth request: %v", err)
		return "", err
	}

	/* get access token */
	resp, err := c.Do(req)
	if err != nil {
		log.Printf("error sending auth request: %v", err)
		return "", err
	}
	defer resp.Body.Close()

	/* unpack */
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("error unpacking body: %v", err)
		return "", err
	}
	parsed, err := url.ParseQuery(string(body))
	if err != nil {
		log.Printf("error parsing URL-encoded response: %v", err)
		return "", err
	}
	accessToken, err := util.GetQueryParam(parsed, "access_token")
	if err != nil {
		return "", err
	}
	return accessToken, nil
}

/* Github account linking */

func (o *AuthService) LinkGithubAccount() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("linking github account...")

		session, ok := r.Context().Value(CtxSessionKey).(*Session)
		if !ok {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}

		linkUrl, err := buildGithubLinkUrl(session.UserID)
		if err != nil {
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		log.Printf("linkUrl: %s\n", linkUrl)

		/* redirect user to GitHub for OAuth linking accounts */
		http.Redirect(w, r, linkUrl, http.StatusFound)
	}
}

func buildGithubLinkUrl(userID int32) (string, error) {
	u, err := url.Parse(ghAuthUrl)
	if err != nil {
		return "", err
	}
	params := url.Values{}
	params.Add("client_id", config.Config.Github.ClientID)
	params.Add("state", strconv.Itoa(int(userID)))
	params.Add("redirect_uri", config.Config.Github.LinkCallback)
	u.RawQuery = params.Encode()
	return u.String(), nil
}

func (o *AuthService) GithubLinkCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		err := o.githubLinkCallback(w, r)
		if err != nil {
			log.Printf("error in githubOAuthLinkGithubAccountCallback: %v", err)
			/* XXX: shouldn't render link option (should show unlink option) but should also
			* show error nicely */
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/user/home", http.StatusTemporaryRedirect)
	}
}

func (o *AuthService) githubLinkCallback(w http.ResponseWriter, r *http.Request) error {
	/* get code */
	queryParams := r.URL.Query()
	code := queryParams.Get("code")
	state := queryParams.Get("state") /* XXX: currently just userID, should make signed to protect against CSRF */

	/* get accessToken */
	accessToken, err := getOauthAccessToken(o.client, code, o.config.ClientID, o.config.ClientSecret)
	if err != nil {
		return err
	}
	/* get user info using token */
	ghUser, err := getGithubUserInfo(o.client, accessToken)
	if err != nil {
		return err
	}
	/* XXX: extract user from state, state == userID currently  */
	uID, err := strconv.ParseInt(state, 10, 32) // base 10, 32 bits
	if err != nil {
		return fmt.Errorf("could not parse userID from state: %w", err)
	}
	/* Validate that user exists */
	userID := int32(uID)
	_, err = o.store.GetUserByID(context.TODO(), userID)
	if err != nil {
		return fmt.Errorf("could not get get user: %w", err)
	}
	/* Link github account to user */
	_, err = o.store.CreateGithubAccount(context.TODO(), model.CreateGithubAccountParams{
		UserID:     userID,
		GhUserID:   ghUser.ID,
		GhEmail:    ghUser.Email,
		GhUsername: ghUser.Username,
	})
	if err != nil {
		return fmt.Errorf("error linking github account to user with ID `%d': %w", userID, err)
	}
	return nil
}

/* json tags for unmarshalling of Github userinfo during OAuth */
type GithubUser struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"login"`
}

func (u *GithubUser) validate() error {
	if u.Email == "" {
		return fmt.Errorf("email must not be empty")
	}
	if u.Username == "" {
		return fmt.Errorf("username must not be empty")
	}
	return nil
}

func getGithubUserInfo(c *http.Client, accessToken string) (GithubUser, error) {
	req, err := util.NewRequestBuilder("GET", ghUserUrl).
		WithHeader("Authorization", fmt.Sprintf("Bearer %s", accessToken)).
		Build()
	if err != nil {
		log.Printf("error building user info request: %v", err)
		return GithubUser{}, err
	}
	/* get user info */
	resp, err := c.Do(req)
	if err != nil {
		return GithubUser{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("error reading response body: %v", err)
		return GithubUser{}, fmt.Errorf("error reading response body: %w", err)
	}
	var user GithubUser
	if err = json.Unmarshal(body, &user); err != nil {
		log.Printf("error unmarshalling JSON: %v", err)
		return GithubUser{}, fmt.Errorf("error unmarshalling JSON: %w", err)
	}
	/* validate user */
	if err = user.validate(); err != nil {
		return GithubUser{}, err
	}
	return user, nil
}

func createAuthSession(userId int32, w http.ResponseWriter, s *model.Store) error {
	sessionId, err := GenerateToken()
	if err != nil {
		return fmt.Errorf("error generating sessionId: %w", err)
	}
	/* check generated sessionId doesn't exist in db */
	_, err = s.GetSession(context.TODO(), sessionId)
	if err == nil {
		return fmt.Errorf("error sessionId already exists")
	} else {
		if err != sql.ErrNoRows {
			return err
		}
	}
	/* create session in db */
	_, err = s.CreateSession(context.TODO(), model.CreateSessionParams{
		Token:  sessionId,
		UserID: userId,
	})
	if err != nil {
		return fmt.Errorf("error writing unauth cookie to db: %w", err)
	}
	/* set cookie */
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    sessionId,
		Path:     "/",
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(168 * time.Hour), /* XXX: make configurable */
	})
	return nil
}

/* Logout */

func (o *AuthService) Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		/* XXX: metrics */

		_, ok := r.Context().Value(CtxSessionKey).(*Session)
		if !ok {
			log.Println("error getting user from context")
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}
		err := o.logout(w, r)
		if err != nil {
			log.Printf("error logging out user: %v", err)
			http.Error(w, "error logging out user", http.StatusInternalServerError)
		}
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
}

func (o *AuthService) logout(w http.ResponseWriter, r *http.Request) error {
	cookie, err := r.Cookie(authCookieName)
	authSessionId := cookie.Value
	if err != nil || authSessionId == "" {
		return fmt.Errorf("error reading auth cookie")
	}
	/* end session */
	err = o.store.EndSession(context.TODO(), cookie.Value)
	if err != nil {
		return err
	}
	/* expire the cookie */
	http.SetCookie(w, &http.Cookie{
		Name:    authCookieName,
		Value:   "",
		Path:    "/",
		Expires: time.Unix(0, 0),
		MaxAge:  -1,
	})
	return nil
}

/* Magic Link Auth */

func (o *AuthService) MagicRegister() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		/* XXX: hardcoding to test linking accounts */

		if err := o.magicRegister(w, r); err != nil {
			log.Printf("error sending register link: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
}

func (o *AuthService) magicRegister(w http.ResponseWriter, r *http.Request) error {
	/* read email parsed through form */
	if err := r.ParseForm(); err != nil {
		/* StatusBadRequest */
		return fmt.Errorf("error parsing form: %w", err)
	}
	emailAddress := r.FormValue("email")
	log.Printf("parsed email `%s' from register form\n", emailAddress)

	/* generate token for register link */
	token, err := GenerateToken()
	if err != nil {
		/* StatusInternalServerError */
		return fmt.Errorf("error generating token: %w", err)
	}
	/* write token to db with email */
	_, err = o.store.CreateMagicRegister(context.TODO(), model.CreateMagicRegisterParams{
		Token: token,
		Email: emailAddress,
	})
	if err != nil {
		return fmt.Errorf("error writing magic to db: %w", err)
	}

	/* send email */
	err = email.SendRegisterLink(o.resendClient, email.MagicLinkParams{
		To:    emailAddress,
		Token: token,
	})
	if err != nil {
		return fmt.Errorf("error sending register link to `%s': %w", emailAddress, err)
	}

	return nil
}

func (o *AuthService) MagicRegisterCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		if err := o.magicRegisterCallback(w, r); err != nil {
			log.Printf("error registering with magic link: %v\n", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		http.Redirect(w, r, "/user/home", http.StatusTemporaryRedirect)
	}
}

func (o *AuthService) magicRegisterCallback(w http.ResponseWriter, r *http.Request) error {
	/* get token from url */
	token := r.URL.Query().Get("token")
	log.Printf("register token `%s'\n", token)

	/* look for magic in db */
	magic, err := o.store.GetMagicRegisterByToken(context.TODO(), token)
	if err != nil {
		return fmt.Errorf("error getting magic by token: %w", err)
	}

	/* create user */
	u, err := o.store.CreateUser(context.TODO(), model.CreateUserParams{
		Email:    magic.Email,
		Username: GenerateUsername(),
	})
	if err != nil {
		return fmt.Errorf("error creating user: %w", err)
	}
	log.Printf("successfully registered user `%v'\n", u)

	/* create Auth Session */
	err = createAuthSession(u.ID, w, o.store)
	if err != nil {
		return fmt.Errorf("error creating auth session: %w", err)
	}
	return nil
}

func (o *AuthService) MagicLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		if err := o.magicLogin(w, r); err != nil {
			log.Printf("error sending login link: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
}

func (o *AuthService) magicLogin(w http.ResponseWriter, r *http.Request) error {
	/* read email parsed through form */
	if err := r.ParseForm(); err != nil {
		/* StatusBadRequest */
		return fmt.Errorf("error parsing login form: %w", err)
	}
	emailAddress := r.FormValue("email")
	log.Printf("parsed email `%s' from login form\n", emailAddress)

	/* generate token for register link */
	token, err := GenerateToken()
	if err != nil {
		/* StatusInternalServerError */
		return fmt.Errorf("error generating token: %w", err)
	}
	/* write token to db with email */
	_, err = o.store.CreateMagicLogin(context.TODO(), model.CreateMagicLoginParams{
		Token: token,
		Email: emailAddress,
	})
	if err != nil {
		return fmt.Errorf("error writing magic login to db: %w", err)
	}
	/* send email */
	err = email.SendLoginLink(o.resendClient, email.MagicLinkParams{
		To:    emailAddress,
		Token: token,
	})
	if err != nil {
		return fmt.Errorf("error sending login link to `%s': %w", emailAddress, err)
	}

	return nil
}

func (o *AuthService) MagicLoginCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		if err := o.magicLoginCallback(w, r); err != nil {
			log.Printf("error logging in with magic link: %v\n", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		http.Redirect(w, r, "/user/home", http.StatusTemporaryRedirect)
	}
}

func (o *AuthService) magicLoginCallback(w http.ResponseWriter, r *http.Request) error {
	/* get token from url */
	token := r.URL.Query().Get("token")
	log.Printf("register token `%s'\n", token)
	/* look for magic in db */
	magic, err := o.store.GetMagicLoginByToken(context.TODO(), token)
	if err != nil {
		return fmt.Errorf("error getting magic by token: %w", err)
	}
	/* create user */
	u, err := o.store.GetUserByEmail(context.TODO(), magic.Email)
	if err != nil {
		return fmt.Errorf("error creating user: %w", err)
	}
	/* create Auth Session */
	err = createAuthSession(u.ID, w, o.store)
	if err != nil {
		return fmt.Errorf("error creating auth session: %w", err)
	}
	log.Printf("successfully logged in user `%v'\n", u)
	return nil
}
