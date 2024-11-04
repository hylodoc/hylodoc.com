package authn

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"text/template"
	"time"

	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack/internal/analytics"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/email"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

const (
	ghAuthUrl  = "https://github.com/login/oauth/authorize"
	ghTokenUrl = "https://github.com/login/oauth/access_token"
	ghUserUrl  = "https://api.github.com/user"

	ghInstallationAccessTokenUrlTemplate = "https://api.github.com/app/installations/%d/access_tokens"
)

var (
	authSessionDuration = time.Now().Add(7 * 24 * time.Hour)
)

type AuthNService struct {
	store        *model.Store
	client       *httpclient.Client
	resendClient *resend.Client
	mixpanel     *analytics.MixpanelClientWrapper
}

func NewAuthNService(
	c *httpclient.Client, resendClient *resend.Client, s *model.Store,
	mixpanel *analytics.MixpanelClientWrapper,
) AuthNService {
	return AuthNService{
		client:       c,
		resendClient: resendClient,
		store:        s,
		mixpanel:     mixpanel,
	}
}

/* Github Auth */

func (a *AuthNService) GithubLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("GithubLogin handler...")

		a.mixpanel.Track("GithubLogin", r)

		authUrl, err := buildGithubOAuthUrl()
		if err != nil {
			logger.Printf("Error in GithubLogin: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		/* redirect user to GitHub for OAuth authorization */

		logger.Println("Redirecting to github for Oauth...")
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

func (a *AuthNService) GithubOAuthCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("GithubOAuthCallback handler...")

		a.mixpanel.Track("GithubOAuthCallback", r)

		/* create user or login user */
		err := a.githubOAuthCallback(w, r)
		if err != nil {
			logger.Printf("Error in OAuthCallback: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		logger.Println("Redirecting user home...")
		http.Redirect(w, r, "/user/", http.StatusTemporaryRedirect)
	}
}

func (a *AuthNService) githubOAuthCallback(
	w http.ResponseWriter, r *http.Request,
) error {
	logger := logging.Logger(r)

	/* get code */
	queryParams := r.URL.Query()
	code := queryParams.Get("code")

	/* get accessToken */
	accessToken, err := getOauthAccessToken(
		a.client,
		code,
		config.Config.Github.ClientID,
		config.Config.Github.ClientSecret,
	)
	if err != nil {
		return err
	}
	/* get user info using token */
	ghUser, err := getGithubUserInfo(a.client, accessToken)
	if err != nil {
		return err
	}

	u, err := a.store.GetUserByGhUserID(context.TODO(), ghUser.ID)
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("error checking for user existence: %w", err)
		}
		/* new user signing in with github, create with github account */
		logger.Println("Creating user and linking to github account in db...")
		err = a.store.CreateUserWithGithubAccountTx(context.TODO(), model.CreateGithubAccountParams{
			GhUserID:   ghUser.ID,
			GhEmail:    ghUser.Email,
			GhUsername: ghUser.Username,
		})
		if err != nil {
			logger.Printf("Error creating user in db: %v", err)
			return fmt.Errorf("error creating user in db: %w", err)
		}
		/* fetch newly created userID, needed to create Auth session */
		u, err = a.store.GetUserByGhUserID(context.TODO(), ghUser.ID)
		if err != nil {
			return fmt.Errorf("error fetching")
		}
	}
	logger.Println("Got user: ", u)

	/* create Auth Session */
	_, err = session.CreateAuthSession(
		a.store, w, u.ID, authSessionDuration, logger,
	)
	if err != nil {
		logger.Printf("Error creating auth session: %v", err)
		return fmt.Errorf("error creating auth session: %w", err)
	}
	return nil
}

func getOauthAccessToken(
	c *httpclient.Client, code, clientID, clientSecret string,
) (string, error) {
	/* build access token request */
	req, err := util.NewRequestBuilder("POST", ghTokenUrl).
		WithHeader("Content-Type", "application/x-www-form-urlencoded").
		WithFormParam("client_id", config.Config.Github.ClientID).
		WithFormParam("client_secret", config.Config.Github.ClientSecret).
		WithFormParam("code", code).
		Build()
	if err != nil {
		return "", fmt.Errorf("error building request: %w", err)
	}

	/* get access token */
	resp, err := c.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	/* unpack */
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}
	parsed, err := url.ParseQuery(string(body))
	if err != nil {
		return "", fmt.Errorf("error parsing query: %w", err)
	}
	accessToken, err := util.GetQueryParam(parsed, "access_token")
	if err != nil {
		return "", fmt.Errorf("error getting accesstoken: %w", err)
	}
	return accessToken, nil
}

/* Github account linking */

func (a *AuthNService) LinkGithubAccount() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("LinkGithubAccount handler...")

		a.mixpanel.Track("LinkGithubAccount", r)

		session, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
			http.Error(w, "", http.StatusNotFound)
			return
		}

		linkUrl, err := buildGithubLinkUrl(session.GetUserID())
		if err != nil {
			logger.Printf("Error building GithubLinkUrl: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		logger.Printf("linkUrl: %s\n", linkUrl)

		/* redirect user to GitHub for OAuth linking accounts */
		logger.Println("Redirecting to Githbub for OAuth linking...")
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

func (a *AuthNService) GithubLinkCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("GithubLinkCallback handler...")

		a.mixpanel.Track("GithubLinkCallback", r)

		err := a.githubLinkCallback(w, r)
		if err != nil {
			logger.Printf("error in githubOAuthLinkGithubAccountCallback: %v", err)
			/* XXX: shouldn't render link option (should show unlink option) but should also
			* show error nicely */
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		logger.Println("Redirecting user home...")
		http.Redirect(w, r, "/user/", http.StatusTemporaryRedirect)
	}
}

func (a *AuthNService) githubLinkCallback(w http.ResponseWriter, r *http.Request) error {
	/* get code */
	queryParams := r.URL.Query()
	code := queryParams.Get("code")
	state := queryParams.Get("state") /* XXX: currently just userID, should make signed to protect against CSRF */

	/* get accessToken */
	accessToken, err := getOauthAccessToken(
		a.client,
		code,
		config.Config.Github.ClientID,
		config.Config.Github.ClientSecret,
	)
	if err != nil {
		return err
	}
	/* get user info using token */
	ghUser, err := getGithubUserInfo(a.client, accessToken)
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
	_, err = a.store.GetUserByID(context.TODO(), userID)
	if err != nil {
		return fmt.Errorf("could not get get user: %w", err)
	}
	/* Link github account to user */
	_, err = a.store.CreateGithubAccount(context.TODO(), model.CreateGithubAccountParams{
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

func getGithubUserInfo(
	c *httpclient.Client, accessToken string,
) (GithubUser, error) {
	req, err := util.NewRequestBuilder("GET", ghUserUrl).
		WithHeader("Authorization", fmt.Sprintf("Bearer %s", accessToken)).
		Build()
	if err != nil {
		return GithubUser{}, fmt.Errorf(
			"error building user info request: %w", err,
		)
	}
	/* get user info */
	resp, err := c.Do(req)
	if err != nil {
		return GithubUser{}, fmt.Errorf(
			"error making request: %w", err,
		)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return GithubUser{}, fmt.Errorf(
			"error reading response body: %w", err,
		)
	}
	var user GithubUser
	if err = json.Unmarshal(body, &user); err != nil {
		return GithubUser{}, fmt.Errorf(
			"error unmarshalling JSON: %w", err,
		)
	}
	/* validate user */
	if err = user.validate(); err != nil {
		return GithubUser{}, fmt.Errorf(
			"error validating user: %w", err,
		)
	}
	return user, nil
}

func (a *AuthNService) Register() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("Register handler...")

		a.mixpanel.Track("Register", r)

		/* get email/username from context */
		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Printf("No Session")
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
			logger,
		)
	}
}

/* Login */

func (a *AuthNService) Login() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("Login handler...")

		a.mixpanel.Track("Login", r)

		/* get email/username from context */
		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
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
			logger,
		)
	}
}

/* Logout */

func (a *AuthNService) Logout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("Logout handler...")

		a.mixpanel.Track("Logout", r)

		_, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
			http.Error(w, "", http.StatusNotFound)
			return
		}
		err := a.logout(w, r)
		if err != nil {
			logger.Printf("Error logging out user: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
		}
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
}

func (a *AuthNService) logout(w http.ResponseWriter, r *http.Request) error {
	logger := logging.Logger(r)

	cookie, err := r.Cookie(session.CookieName)
	authSessionId := cookie.Value
	if err != nil || authSessionId == "" {
		return fmt.Errorf("error reading auth cookie")
	}
	return session.EndAuthSession(a.store, w, authSessionId, logger)
}

/* Magic Link Auth */

func (a *AuthNService) MagicRegister() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("MagicRegister handler...")

		a.mixpanel.Track("MagicRegister", r)

		if err := a.magicRegister(w, r); err != nil {
			logger.Printf("error sending register link: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
}

func (a *AuthNService) magicRegister(w http.ResponseWriter, r *http.Request) error {
	logger := logging.Logger(r)

	/* read email parsed through form */
	if err := r.ParseForm(); err != nil {
		/* StatusBadRequest */
		return fmt.Errorf("error parsing form: %w", err)
	}
	emailAddress := r.FormValue("email")
	logger.Printf("parsed email `%s' from register form\n", emailAddress)

	/* generate token for register link */
	token, err := GenerateToken()
	if err != nil {
		/* StatusInternalServerError */
		return fmt.Errorf("error generating token: %w", err)
	}
	/* write token to db with email */
	_, err = a.store.CreateMagicRegister(context.TODO(), model.CreateMagicRegisterParams{
		Token: token,
		Email: emailAddress,
	})
	if err != nil {
		return fmt.Errorf("error writing magic to db: %w", err)
	}

	/* send email */
	err = email.SendRegisterLink(a.resendClient, email.MagicLinkParams{
		To:    emailAddress,
		Token: token,
	})
	if err != nil {
		return fmt.Errorf("error sending register link to `%s': %w", emailAddress, err)
	}
	return nil
}

func (a *AuthNService) MagicRegisterCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("MagicRegisterCallback handler...")

		a.mixpanel.Track("MagicRegisterCallback", r)

		if err := a.magicRegisterCallback(w, r); err != nil {
			logger.Printf("Error registering with magic link: %v\n", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		logger.Println("Redirecting user to home...")
		http.Redirect(w, r, "/user/", http.StatusTemporaryRedirect)
	}
}

func (a *AuthNService) magicRegisterCallback(w http.ResponseWriter, r *http.Request) error {
	logger := logging.Logger(r)

	/* get token from url */
	token := r.URL.Query().Get("token")
	logger.Printf("register token `%s'\n", token)

	/* look for magic in db */
	magic, err := a.store.GetMagicRegisterByToken(context.TODO(), token)
	if err != nil {
		return fmt.Errorf("error getting magic by token: %w", err)
	}

	/* create user */
	u, err := a.store.CreateUserTx(context.TODO(), model.CreateUserParams{
		Email:    magic.Email,
		Username: GenerateUsername(),
	})
	if err != nil {
		return fmt.Errorf("error creating user: %w", err)
	}
	logger.Printf("Successfully registered user `%v'\n", u)

	/* create Auth Session */
	_, err = session.CreateAuthSession(
		a.store, w, u.ID, authSessionDuration, logger,
	)
	if err != nil {
		return fmt.Errorf("error creating auth session: %w", err)
	}
	return nil
}

func (a *AuthNService) MagicLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("MagicLogin handler...")

		a.mixpanel.Track("MagicLogin", r)

		if err := a.magicLogin(w, r); err != nil {
			logger.Printf("Error sending login link: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
	}
}

func (a *AuthNService) magicLogin(w http.ResponseWriter, r *http.Request) error {
	/* read email parsed through form */
	if err := r.ParseForm(); err != nil {
		/* StatusBadRequest */
		return fmt.Errorf("error parsing login form: %w", err)
	}
	emailAddress := r.FormValue("email")

	/* generate token for register link */
	token, err := GenerateToken()
	if err != nil {
		/* StatusInternalServerError */
		return fmt.Errorf("error generating token: %w", err)
	}
	/* write token to db with email */
	_, err = a.store.CreateMagicLogin(context.TODO(), model.CreateMagicLoginParams{
		Token: token,
		Email: emailAddress,
	})
	if err != nil {
		return fmt.Errorf("error writing magic login to db: %w", err)
	}
	/* send email */
	err = email.SendLoginLink(a.resendClient, email.MagicLinkParams{
		To:    emailAddress,
		Token: token,
	})
	if err != nil {
		return fmt.Errorf("error sending login link to `%s': %w", emailAddress, err)
	}

	return nil
}

func (a *AuthNService) MagicLoginCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("MagicLoginCallback...")

		a.mixpanel.Track("MagicLoginCallback", r)

		if err := a.magicLoginCallback(w, r); err != nil {
			logger.Printf(
				"error logging in with magic link: %v\n", err,
			)
			http.Error(w, "", http.StatusUnauthorized)
			return
		}

		logger.Println("Redirecting user to home...")
		http.Redirect(w, r, "/user/", http.StatusTemporaryRedirect)
	}
}

func (a *AuthNService) magicLoginCallback(w http.ResponseWriter, r *http.Request) error {
	logger := logging.Logger(r)

	/* get token from url */
	token := r.URL.Query().Get("token")
	logger.Printf("register token `%s'\n", token)
	/* look for magic in db */
	magic, err := a.store.GetMagicLoginByToken(context.TODO(), token)
	if err != nil {
		return fmt.Errorf("error getting magic by token: %w", err)
	}
	/* create user */
	u, err := a.store.GetUserByEmail(context.TODO(), magic.Email)
	if err != nil {
		return fmt.Errorf("error creating user: %w", err)
	}
	/* create Auth Session */
	_, err = session.CreateAuthSession(
		a.store, w, u.ID, authSessionDuration, logger,
	)
	if err != nil {
		return fmt.Errorf("error creating auth session: %w", err)
	}
	logger.Printf("Successfully logged in user `%v'\n", u)
	return nil
}
