package authn

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/billing"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/email"
	"github.com/xr0-org/progstack/internal/email/emailaddr"
	"github.com/xr0-org/progstack/internal/httpclient"
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
	store  *model.Store
	client *httpclient.Client
}

func NewAuthNService(
	c *httpclient.Client, s *model.Store,
) AuthNService {
	return AuthNService{
		client: c,
		store:  s,
	}
}

/* Github Auth */

func (a *AuthNService) GithubLogin(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("GithubLogin handler...")
	r.MixpanelTrack("GithubLogin")
	authUrl, err := buildGithubOAuthUrl()
	if err != nil {
		return nil, fmt.Errorf("GithubLogin: %w", err)
	}
	/* redirect user to GitHub for OAuth authorization */
	sesh.Println("Redirecting to github for Oauth...")
	return response.NewRedirect(authUrl, http.StatusFound), nil
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

func (a *AuthNService) GithubOAuthCallback(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("GithubOAuthCallback handler...")
	r.MixpanelTrack("GithubOAuthCallback")
	/* create user or login user */
	if err := a.githubOAuthCallback(r); err != nil {
		return nil, fmt.Errorf("OAuthCallback: %w", err)
	}
	sesh.Println("Redirecting user home...")
	return response.NewRedirect("/user/", http.StatusTemporaryRedirect), nil
}

func (a *AuthNService) githubOAuthCallback(r request.Request) error {
	sesh := r.Session()

	/* get accessToken */
	accessToken, err := getOauthAccessToken(
		a.client,
		r.GetURLQueryValue("code"),
		config.Config.Github.ClientID,
		config.Config.Github.ClientSecret,
	)
	if err != nil {
		return fmt.Errorf("get access token: %w", err)
	}
	u, err := a.getOrCreateGhUser(accessToken, sesh)
	if err != nil {
		return fmt.Errorf("get or create user: %w", err)
	}
	sesh.Printf("got user: %v\n", u)

	/* create Auth Session */
	if _, err := sesh.Authenticate(
		a.store, r.ResponseWriter(), u.ID, authSessionDuration,
	); err != nil {
		return fmt.Errorf("error creating auth session: %w", err)
	}

	return nil
}

func (a *AuthNService) getOrCreateGhUser(
	token string, sesh *session.Session,
) (*model.User, error) {
	ghUser, err := getGithubUserInfo(a.client, token)
	if err != nil {
		return nil, fmt.Errorf("get github user info: %w", err)
	}
	u, err := a.store.GetUserByGhUserID(context.TODO(), ghUser.ID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf(
				"check for user existence: %w", err,
			)
		}
		/* new user signing in with github, create with github account */
		sesh.Println("creating user and linking to github account in db...")
		newu, err := createUserWithGithubAccount(
			&model.CreateGithubAccountParams{
				GhUserID:   ghUser.ID,
				GhEmail:    ghUser.Email,
				GhUsername: ghUser.Username,
			},
			a.store,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"create user with github account: %w", err,
			)
		}
		/* autosubscribe user to stripe */
		if err = billing.AutoSubscribeToFreePlan(
			newu, a.store, sesh,
		); err != nil {
			return nil, fmt.Errorf(
				"auto subscribe to free plan: %w", err,
			)
		}
		return newu, nil
	}
	return &u, nil
}

func createUserWithGithubAccount(
	arg *model.CreateGithubAccountParams, s *model.Store,
) (*model.User, error) {
	var res *model.User
	if err := s.ExecTx(
		func(tx *model.Store) error {
			u, err := createUserWithGithubAccountTx(arg, tx)
			if err != nil {
				return err
			}
			res = u
			return nil
		},
	); err != nil {
		return nil, err
	}
	assert.Assert(res != nil)
	return res, nil
}

func createUserWithGithubAccountTx(
	arg *model.CreateGithubAccountParams, tx *model.Store,
) (*model.User, error) {
	u, err := tx.CreateUser(
		context.TODO(),
		model.CreateUserParams{
			Email:    arg.GhEmail,
			Username: arg.GhUsername, /* we use github username */
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	if _, err := tx.CreateGithubAccount(
		context.TODO(),
		model.CreateGithubAccountParams{
			UserID:     u.ID,
			GhUserID:   arg.GhUserID,
			GhEmail:    arg.GhEmail,
			GhUsername: arg.GhUsername,
		},
	); err != nil {
		return nil, fmt.Errorf("create github account: %w", err)
	}
	return &u, nil
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

func (a *AuthNService) LinkGithubAccount(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("LinkGithubAccount handler...")

	r.MixpanelTrack("LinkGithubAccount")
	userid, err := sesh.GetUserID()
	if err != nil {
		return nil, fmt.Errorf("get user id: %w", err)
	}
	linkUrl, err := buildGithubLinkUrl(userid)
	if err != nil {
		return nil, fmt.Errorf("GithubLinkUrl: %w", err)
	}
	sesh.Printf("linkUrl: %s\n", linkUrl)

	/* redirect user to GitHub for OAuth linking accounts */
	sesh.Println("Redirecting to Githbub for OAuth linking...")
	return response.NewRedirect(linkUrl, http.StatusFound), nil
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

func (a *AuthNService) GithubLinkCallback(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("GithubLinkCallback handler...")

	r.MixpanelTrack("GithubLinkCallback")

	if err := a.githubLinkCallback(r); err != nil {
		/* XXX: shouldn't render link option (should show unlink
		 * option) but should also show error nicely */
		return nil, fmt.Errorf("githubOAuthLinkCallback: %w", err)
	}
	sesh.Println("Redirecting user home...")
	return response.NewRedirect("/user/", http.StatusTemporaryRedirect), nil
}

func (a *AuthNService) githubLinkCallback(r request.Request) error {
	/* get accessToken */
	accessToken, err := getOauthAccessToken(
		a.client,
		r.GetURLQueryValue("code"),
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
	uID, err := strconv.ParseInt(r.GetURLQueryValue("state"), 10, 32)
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

func (a *AuthNService) Register(r request.Request) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("Register handler...")
	r.MixpanelTrack("Register")
	return response.NewTemplate(
		[]string{"register.html"},
		util.PageInfo{
			Data: struct {
				Title    string
				UserInfo *session.UserInfo
			}{
				Title:    "Progstack - blogging for devs",
				UserInfo: session.ConvertSessionToUserInfo(r.Session()),
			},
		},
	), nil
}

/* Login */

func (a *AuthNService) Login(r request.Request) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("Login handler...")
	r.MixpanelTrack("Login")

	return response.NewTemplate([]string{"login.html"},
		util.PageInfo{
			Data: struct {
				Title    string
				UserInfo *session.UserInfo
			}{
				Title: "Progstack - blogging for devs",
				UserInfo: session.ConvertSessionToUserInfo(
					r.Session(),
				),
			},
		},
	), nil
}

/* Logout */

func (a *AuthNService) Logout(r request.Request) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("Logout handler...")

	r.MixpanelTrack("Logout")
	if err := r.Session().End(a.store); err != nil {
		return nil, fmt.Errorf("end session: %w", err)
	}
	return response.NewRedirect("/", http.StatusTemporaryRedirect), nil
}

/* Magic Link Auth */

func (a *AuthNService) MagicRegister(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("MagicRegister handler...")
	r.MixpanelTrack("MagicRegister")

	if err := a.magicRegister(r); err != nil {
		return nil, fmt.Errorf("magic register: %w", err)
	}
	return response.NewRedirect("/", http.StatusTemporaryRedirect), nil
}

func (a *AuthNService) magicRegister(r request.Request) error {
	/* read email parsed through form */
	toaddr, err := r.GetFormValue("email")
	if err != nil {
		return fmt.Errorf("get email: %w", err)
	}

	/* generate token for register link */
	token, err := GenerateToken()
	if err != nil {
		/* StatusInternalServerError */
		return fmt.Errorf("error generating token: %w", err)
	}
	if _, err := a.store.CreateMagicRegister(
		context.TODO(),
		model.CreateMagicRegisterParams{
			Token: token,
			Email: toaddr,
		},
	); err != nil {
		return fmt.Errorf("error writing magic to db: %w", err)
	}

	if err := email.NewSender(
		emailaddr.NewAddr(toaddr),
		emailaddr.NewAddr(
			fmt.Sprintf(
				"magic@%s", config.Config.Progstack.EmailDomain,
			),
		),
		model.EmailModePlaintext,
		a.store,
	).SendRegisterLink(token); err != nil {
		return fmt.Errorf(
			"error sending register link to `%s': %w",
			toaddr, err,
		)
	}
	return nil
}

func (a *AuthNService) MagicRegisterCallback(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("MagicRegisterCallback handler...")
	r.MixpanelTrack("MagicRegisterCallback")

	if err := a.magicRegisterCallback(r); err != nil {
		return nil, fmt.Errorf("magic register callback: %w", err)
	}
	sesh.Println("Redirecting user to home...")
	return response.NewRedirect("/", http.StatusTemporaryRedirect), nil
}

func (a *AuthNService) magicRegisterCallback(r request.Request) error {
	sesh := r.Session()

	/* look for magic in db */
	magic, err := a.store.GetMagicRegisterByToken(
		context.TODO(), r.GetURLQueryValue("token"),
	)
	if err != nil {
		return fmt.Errorf("error getting magic by token: %w", err)
	}

	u, err := a.store.CreateUser(context.TODO(), model.CreateUserParams{
		Email:    magic.Email,
		Username: GenerateUsername(),
	})
	if err != nil {
		return fmt.Errorf("error creating user: %w", err)
	}
	sesh.Printf("Successfully registered user `%v'\n", u)

	/* autosubscribe user to stripe */
	if err = billing.AutoSubscribeToFreePlan(&u, a.store, sesh); err != nil {
		return fmt.Errorf("error subscribing user to free plan: %w", err)
	}

	/* create Auth Session */
	if _, err := sesh.Authenticate(
		a.store, r.ResponseWriter(), u.ID, authSessionDuration,
	); err != nil {
		return fmt.Errorf("error creating auth session: %w", err)
	}
	return nil
}

func (a *AuthNService) MagicLogin(r request.Request) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("MagicLogin handler...")
	r.MixpanelTrack("MagicLogin")

	if err := a.magicLogin(r); err != nil {
		return nil, fmt.Errorf("magic login: %w", err)
	}
	return response.NewRedirect("/", http.StatusTemporaryRedirect), nil
}

func (a *AuthNService) magicLogin(r request.Request) error {
	/* read email parsed through form */
	toaddr, err := r.GetFormValue("email")
	if err != nil {
		return fmt.Errorf("get email: %w", err)
	}

	/* generate token for register link */
	token, err := GenerateToken()
	if err != nil {
		/* StatusInternalServerError */
		return fmt.Errorf("error generating token: %w", err)
	}
	if _, err := a.store.CreateMagicLogin(
		context.TODO(),
		model.CreateMagicLoginParams{
			Token: token,
			Email: toaddr,
		},
	); err != nil {
		return fmt.Errorf("error writing magic login to db: %w", err)
	}

	if err := email.NewSender(
		emailaddr.NewAddr(toaddr),
		emailaddr.NewAddr(
			fmt.Sprintf(
				"magic@%s", config.Config.Progstack.EmailDomain,
			),
		),
		model.EmailModePlaintext,
		a.store,
	).SendLoginLink(token); err != nil {
		return fmt.Errorf(
			"error sending login link to `%s': %w",
			toaddr, err,
		)
	}
	return nil
}

func (a *AuthNService) MagicLoginCallback(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("MagicLoginCallback...")
	r.MixpanelTrack("MagicLoginCallback")

	if err := a.magicLoginCallback(r); err != nil {
		return nil, fmt.Errorf("magic login callback: %w", err)
	}
	sesh.Println("Redirecting user to home...")
	return response.NewRedirect("/user/", http.StatusTemporaryRedirect), nil
}

func (a *AuthNService) magicLoginCallback(r request.Request) error {
	/* look for magic in db */
	magic, err := a.store.GetMagicLoginByToken(
		context.TODO(), r.GetURLQueryValue("token"),
	)
	if err != nil {
		return fmt.Errorf("error getting magic by token: %w", err)
	}
	/* create user */
	u, err := a.store.GetUserByEmail(context.TODO(), magic.Email)
	if err != nil {
		return fmt.Errorf("error creating user: %w", err)
	}
	/* create Auth Session */
	sesh := r.Session()
	if _, err := sesh.Authenticate(
		a.store, r.ResponseWriter(), u.ID, authSessionDuration,
	); err != nil {
		return fmt.Errorf("error creating auth session: %w", err)
	}
	sesh.Printf("Successfully logged in user `%v'\n", u)
	return nil
}
