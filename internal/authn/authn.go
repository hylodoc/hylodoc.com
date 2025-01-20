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
	"time"

	"github.com/knuthic/knuthic/internal/app/handler/request"
	"github.com/knuthic/knuthic/internal/app/handler/response"
	"github.com/knuthic/knuthic/internal/billing"
	"github.com/knuthic/knuthic/internal/config"
	"github.com/knuthic/knuthic/internal/httpclient"
	"github.com/knuthic/knuthic/internal/model"
	"github.com/knuthic/knuthic/internal/session"
	"github.com/knuthic/knuthic/internal/util"
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
		newu, err := a.store.CreateUser(
			context.TODO(),
			model.CreateUserParams{
				Email:    ghUser.Email,
				Username: ghUser.Username,
				GhUserID: ghUser.ID,
			},
		)
		if err != nil {
			return nil, fmt.Errorf(
				"create user with github account: %w", err,
			)
		}
		/* autosubscribe user to stripe */
		if err = billing.AutoSubscribeToFreePlan(
			&newu, a.store, sesh,
		); err != nil {
			return nil, fmt.Errorf(
				"auto subscribe to free plan: %w", err,
			)
		}
		return &newu, nil
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
				Title:    "Knuthic - blogging for devs",
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
				Title: "Knuthic - blogging for devs",
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
