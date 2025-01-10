package user

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/authz"
	"github.com/xr0-org/progstack/internal/blog"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/installation"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

type UserService struct {
	store *model.Store
}

func NewUserService(s *model.Store) *UserService {
	return &UserService{s}
}

func (u *UserService) Home(r request.Request) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("Home handler...")

	r.MixpanelTrack("Home")

	/* get session */
	userid, err := sesh.GetUserID()
	if err != nil {
		return nil, fmt.Errorf("get user id: %w", err)
	}
	blogs, err := blog.GetBlogsInfo(u.store, userid)
	if err != nil {
		return nil, fmt.Errorf("blogs: %w", err)
	}

	githubInstallAppUrl := fmt.Sprintf(
		installation.GhInstallUrlTemplate,
		config.Config.Github.AppName,
	)
	return response.NewTemplate(
		[]string{"home.html", "blogs.html"},
		util.PageInfo{
			Data: struct {
				Title               string
				UserInfo            *session.UserInfo
				GithubInstallAppUrl string
				Blogs               []blog.BlogInfo
			}{
				Title:               "Home",
				UserInfo:            session.ConvertSessionToUserInfo(sesh),
				GithubInstallAppUrl: githubInstallAppUrl,
				Blogs:               blogs,
			},
		},
	), nil
}

func (u *UserService) GithubInstallation(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Printf("GithubInstallation handler...")

	r.MixpanelTrack("GithubInstallation")

	userid, err := sesh.GetUserID()
	if err != nil {
		return nil, fmt.Errorf("get user id: %w", err)
	}
	if err := u.store.UpdateAwaitingGithubUpdate(
		context.TODO(),
		model.UpdateAwaitingGithubUpdateParams{
			ID:               userid,
			GhAwaitingUpdate: true,
		},
	); err != nil {
		return nil, fmt.Errorf("error updating awaiting: %w", err)
	}

	return response.NewRedirect(
		fmt.Sprintf(
			installation.GhInstallUrlTemplate,
			config.Config.Github.AppName,
		),
		http.StatusTemporaryRedirect,
	), nil
}

type AccountDetails struct {
	Username        string
	Email           string
	IsLinked        bool
	HasInstallation bool
	GithubEmail     string
	Subscription    Subscription
	StorageUsed     string
	StorageLimit    string
}

type StorageDetails struct {
	Used  string
	Limit string
}

type Subscription struct {
	Plan               string
	CurrentPeriodStart string
	CurrentPeriodEnd   string
	Amount             string
}

func (u *UserService) Account(r request.Request) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("Account handler...")

	r.MixpanelTrack("Account")

	accountDetails, err := getAccountDetails(u.store, sesh)
	if err != nil {
		return nil, fmt.Errorf("account details: %w", err)
	}
	storageDetails, err := getStorageDetails(u.store, sesh)
	if err != nil {
		return nil, fmt.Errorf("storage details: %w", err)
	}

	return response.NewTemplate(
		[]string{"account.html"},
		util.PageInfo{
			Data: struct {
				Title          string
				UserInfo       *session.UserInfo
				AccountDetails AccountDetails
				StorageDetails StorageDetails
			}{
				Title:          "Home",
				UserInfo:       session.ConvertSessionToUserInfo(sesh),
				AccountDetails: accountDetails,
				StorageDetails: storageDetails,
			},
		},
	), nil
}

func getStorageDetails(s *model.Store, sesh *session.Session) (StorageDetails, error) {
	/* calculate storage */
	userid, err := sesh.GetUserID()
	if err != nil {
		return StorageDetails{}, fmt.Errorf("get user id: %w", err)
	}
	userBytes, err := authz.UserStorageUsed(s, userid)
	if err != nil {
		return StorageDetails{}, err
	}
	userMegaBytes := float64(userBytes) / (1024 * 1024)

	/* XXX: plan limits details */
	return StorageDetails{
		Used:  fmt.Sprintf("%.2f", userMegaBytes),
		Limit: "10",
	}, nil
}

func getAccountDetails(s *model.Store, sesh *session.Session) (AccountDetails, error) {
	/* get github info */
	accountDetails := AccountDetails{
		Username:        sesh.GetUsername(),
		Email:           sesh.GetEmail(),
		IsLinked:        false,
		HasInstallation: false,
		GithubEmail:     "",
	}
	linked := true
	userid, err := sesh.GetUserID()
	if err != nil {
		return AccountDetails{}, fmt.Errorf("get user id: %w", err)
	}
	ghAccount, err := s.GetGithubAccountByUserID(context.TODO(), userid)
	if err != nil {
		if err != sql.ErrNoRows {
			return AccountDetails{}, fmt.Errorf(
				"error getting account details: %w", err,
			)
		}
		/* no linked Github account*/
		linked = false
	}
	if linked {
		accountDetails.IsLinked = true
		accountDetails.GithubEmail = ghAccount.GhEmail
	}

	hasInstallation, err := s.InstallationExistsForUserID(
		context.TODO(), userid,
	)
	if err != nil {
		return AccountDetails{}, fmt.Errorf(
			"error checking if user has installation: %w", err,
		)
	}
	if hasInstallation {
		accountDetails.HasInstallation = true
	}

	/* get stripe subscription */
	sub, err := s.GetStripeSubscriptionByUserID(context.TODO(), userid)
	if err != nil {
		return AccountDetails{}, fmt.Errorf(
			"error getting stripe subscription details: %w",
			err,
		)
	}
	sesh.Printf("subName: %s", sub.SubName)

	accountDetails.Subscription = Subscription{
		Plan: string(sub.SubName),
	}
	return accountDetails, nil
}
