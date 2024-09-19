package auth

import (
	"context"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/xr0-org/progstack/internal/config"
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

/* AuthMiddleware */

type AuthMiddleware struct {
	store *model.Store
}

func NewAuthMiddleware(s *model.Store) *AuthMiddleware {
	return &AuthMiddleware{
		store: s,
	}
}

func (a *AuthMiddleware) ValidateAuthSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("running auth session middleware...")
		session, err := validateAuthSession(w, r, a.store)
		if err != nil {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		ctx := context.WithValue(r.Context(), CtxSessionKey, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func validateAuthSession(w http.ResponseWriter, r *http.Request, s *model.Store) (*Session, error) {
	cookie, err := r.Cookie(authCookieName)
	authSessionId := cookie.Value
	if err != nil || authSessionId == "" {
		return nil, fmt.Errorf("error reading auth cookie")
	}
	session, err := validateAuthSessionId(authSessionId, w, s)
	if err != nil {
		log.Println("error validating authSessionId: ", err)
		return nil, fmt.Errorf("error validating auth session id: %w", err)
	}
	return session, nil
}

type Session struct {
	UserID   int32  `json:"user_id"`
	Email    string `json:"email"`
	Username string `json:"login"`
}

func validateAuthSessionId(sessionId string, w http.ResponseWriter, s *model.Store) (*Session, error) {
	session, err := s.GetSession(context.TODO(), sessionId)
	if err != nil {
		if err != sql.ErrNoRows {
			/* db error */
			return nil, err
		}
		/* no auth session exists, delete auth cookie */
		http.SetCookie(w, &http.Cookie{
			Name:    authCookieName,
			Value:   "",
			Path:    "/",
			Expires: time.Unix(0, 0),
			MaxAge:  -1,
		})
		return nil, err
	}
	if session.ExpiresAt.Before(time.Now()) {
		log.Println("auth token expired")
		/* expired session in db */
		err := s.EndSession(context.TODO(), sessionId)
		if err != nil {
			return nil, err
		}
		/* delete cookie */
		http.SetCookie(w, &http.Cookie{
			Name:    authCookieName,
			Value:   "",
			Path:    "/",
			Expires: time.Unix(0, 0),
			MaxAge:  -1,
		})
		return nil, fmt.Errorf("session expired")
	}
	return &Session{
		UserID:   session.UserID,
		Username: session.Username,
		Email:    session.Email,
	}, nil
}

/* HandleAuth */

type AuthService struct {
	store  *model.Store
	client *http.Client
	config *config.GithubParams
}

func NewAuthService(c *http.Client, s *model.Store, config *config.GithubParams) AuthService {
	return AuthService{
		client: c,
		store:  s,
		config: config,
	}
}

func (o *AuthService) Login() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authUrl, err := buildAuthUrl(o.config.ClientID)
		if err != nil {
			log.Printf("error in login: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		/* redirect user to GitHub for OAuth authorization */
		http.Redirect(w, r, authUrl, http.StatusFound)
	}
}

func buildAuthUrl(ClientID string) (string, error) {
	u, err := url.Parse(ghAuthUrl)
	if err != nil {
		return "", err
	}
	params := url.Values{}
	params.Add("client_id", ClientID)
	u.RawQuery = params.Encode()
	return u.String(), nil
}

func (o *AuthService) OAuthCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		/* create user or login user */
		/* XXX: these db accesses to end of func should all be atomic in a Tx */
		err := o.oauthCallbackProcess(w, r)
		if err != nil {
			log.Printf("error in OAuthCallback: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, "/home", http.StatusTemporaryRedirect)
	}
}

func (o *AuthService) oauthCallbackProcess(w http.ResponseWriter, r *http.Request) error {
	/* get code */
	queryParams := r.URL.Query()
	code := queryParams.Get("code")

	/* get accessToken */
	accessToken, err := getOauthAccessToken(o.client, code, o.config.ClientID, o.config.ClientSecret)
	if err != nil {
		return err
	}
	/* build user info request */
	user, err := getUserInfo(o.client, accessToken)
	if err != nil {
		return err
	}
	/* handle user */
	u, err := o.store.GetUserByGithubId(context.TODO(), user.UserId)
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("error checking for user existence: %w", err)
		}
		log.Println("creating user in db...")
		req := model.CreateUserParams{
			GhUserID: user.UserId, Email: user.Email, Username: user.Username,
		}
		u, err = o.store.CreateUser(context.TODO(), req)
		if err != nil {
			log.Printf("error creating user in db: %v", err)
			return fmt.Errorf("error creating user in db: %w", err)
		}
	}
	log.Println("got user: ", u)

	/* create Auth Session */
	err = createAuthSession(u.ID, w, o.store)
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

/* json tags for unmarshalling of Github userinfo during OAuth */
type User struct {
	UserId   int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"login"`
}

func getUserInfo(c *http.Client, accessToken string) (User, error) {
	req, err := util.NewRequestBuilder("GET", ghUserUrl).
		WithHeader("Authorization", fmt.Sprintf("Bearer %s", accessToken)).
		Build()
	if err != nil {
		log.Printf("error building user info request: %v", err)
		return User{}, err
	}
	/* get user info */
	resp, err := c.Do(req)
	if err != nil {
		return User{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("error reading response body: %v", err)
		return User{}, fmt.Errorf("error reading response body: %w", err)
	}
	var user User
	if err = json.Unmarshal(body, &user); err != nil {
		log.Printf("error unmarshalling JSON: %v", err)
		return User{}, fmt.Errorf("error unmarshalling JSON: %w", err)
	}
	return user, nil
}

func createAuthSession(userId int32, w http.ResponseWriter, s *model.Store) error {
	sessionId, err := generateToken()
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
	/* create unauthSession in db */
	_, err = s.CreateSession(context.TODO(), model.CreateSessionParams{
		Token: sessionId, UserID: userId,
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

/* Installation AccessToken */

type InstallationAccessToken struct {
	Token     string    `json:"token"`      /* The installation access token */
	ExpiresAt time.Time `json:"expires_at"` /* The timestamp when the token expires */
}

func GetInstallationAccessToken(client *http.Client, appID, installationID int64, privateKeyPath string) (string, error) {
	jwt, err := createJWT(appID, privateKeyPath)
	if err != nil {
		return "", fmt.Errorf("Error creating JWT: %w", err)
	}

	url := fmt.Sprintf(ghInstallationAccessTokenUrlTemplate, installationID)
	req, err := util.NewRequestBuilder("POST", url).
		WithHeader("Authorization", fmt.Sprintf("Bearer %s", jwt)).
		WithHeader("Accept", "application/vnd.github+json").
		WithHeader("X-GitHub-Api-Version", "2022-11-28").
		Build()
	if err != nil {
		return "", err
	}
	log.Printf("AccessTokenRequest: %+v", req)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	log.Printf("resp: %+v", resp)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	log.Printf("body: %+v", body)
	var accessToken InstallationAccessToken
	err = json.Unmarshal(body, &accessToken)
	if err != nil {
		return "", err
	}
	log.Printf("accessToken: %+v", accessToken)
	return accessToken.Token, nil
}

func createJWT(appID int64, privateKeyPath string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Add(-60 * time.Second).Unix(), /* allow for clock drift per docs */
		"exp": now.Add(10 * time.Minute).Unix(),
		"iss": appID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	privateKey, err := loadPrivateKey(privateKeyPath)
	if err != nil {
		return "", fmt.Errorf("Error loading private key: %w", err)
	}
	jwtToken, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("error signing token with private key: %w", err)
	}
	err = validateJWT(jwtToken, &privateKey.PublicKey)
	if err != nil {
		return "", fmt.Errorf("error validating jwt token: %w", err)
	}
	return jwtToken, nil
}

func loadPrivateKey(filePath string) (*rsa.PrivateKey, error) {
	keyData, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error parsing private key: %w", err)
	}
	return jwt.ParseRSAPrivateKeyFromPEM(keyData)
}

func validateJWT(tokenString string, publicKey *rsa.PublicKey) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Ensure the signing method is RS256
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})
	if err != nil {
		return err
	}
	if !token.Valid {
		return fmt.Errorf("invalid token")
	}
	return nil
}
