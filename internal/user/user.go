package user

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

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
	logger := r.Logger()
	logger.Println("Home handler...")

	r.MixpanelTrack("Home")

	/* get session */
	sesh := r.Session()
	blogs, err := blog.GetBlogsInfo(u.store, sesh.GetUserID())
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
		template.FuncMap{},
		logger,
	), nil
}

func (u *UserService) CreateNewBlog(r request.Request) (response.Response, error) {
	logger := r.Logger()
	logger.Println("CreateNewBlog handler...")

	r.MixpanelTrack("CreateNewBlog")

	sesh := r.Session()
	if err := authz.CanCreateSite(u.store, sesh); err != nil {
		return nil, fmt.Errorf("CanCreateSite: %w", err)
	}

	return response.NewTemplate(
		[]string{"blog_create.html"},
		util.PageInfo{
			Data: struct {
				Title    string
				UserInfo *session.UserInfo
			}{
				Title:    "Create New Blog",
				UserInfo: session.ConvertSessionToUserInfo(sesh),
			},
		},
		template.FuncMap{},
		logger,
	), nil
}

func (u *UserService) FolderFlow(r request.Request) (response.Response, error) {
	logger := r.Logger()
	logger.Printf("FolderFlow handler...")

	r.MixpanelTrack("FolderFlow")

	return response.NewTemplate(
		[]string{"blog_folder_flow.html"},
		util.PageInfo{
			Data: struct {
				Title       string
				UserInfo    *session.UserInfo
				ServiceName string
				Themes      []string
			}{
				Title:       "Folder Flow",
				UserInfo:    session.ConvertSessionToUserInfo(r.Session()),
				ServiceName: config.Config.Progstack.ServiceName,
				Themes:      blog.BuildThemes(config.Config.ProgstackSsg.Themes),
			},
		},
		template.FuncMap{},
		logger,
	), nil
}

func (u *UserService) GithubInstallation(
	r request.Request,
) (response.Response, error) {
	logger := r.Logger()
	logger.Printf("GithubInstallation handler...")

	r.MixpanelTrack("GithubInstallation")

	if err := u.store.UpdateAwaitingGithubUpdate(
		context.TODO(),
		model.UpdateAwaitingGithubUpdateParams{
			ID:               r.Session().GetUserID(),
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

func (u *UserService) awaitupdate(userID int32) error {
	/* TODO: get from config */
	var (
		timeout = 5 * time.Second
		step    = 100 * time.Millisecond
	)
	for until := time.Now().Add(timeout); time.Now().Before(until); time.Sleep(step) {
		awaiting, err := u.store.IsAwaitingGithubUpdate(
			context.TODO(), userID,
		)
		if err != nil {
			return fmt.Errorf("error checking if awaiting: %w", err)
		}
		if !awaiting {
			return nil
		}
	}
	if err := u.store.UpdateAwaitingGithubUpdate(
		context.TODO(),
		model.UpdateAwaitingGithubUpdateParams{
			ID:               userID,
			GhAwaitingUpdate: false,
		},
	); err != nil {
		return fmt.Errorf("error updating awaitingGithubUpdate: %w", err)
	}
	return fmt.Errorf("timeout")
}

type Repository struct {
	Value int64
	Name  string
}

func (u *UserService) RepositoryFlow(
	r request.Request,
) (response.Response, error) {
	logger := r.Logger()
	logger.Println("RepositoryFlow handler...")

	r.MixpanelTrack("RepositoryFlow")

	if err := u.awaitupdate(r.Session().GetUserID()); err != nil {
		return nil, fmt.Errorf("await update: %w", err)
	}

	sesh := r.Session()
	repos, err := u.store.ListOrderedRepositoriesByUserID(
		context.TODO(), sesh.GetUserID(),
	)
	if err != nil {
		return nil, fmt.Errorf("get repositories: %w", err)
	}

	details, err := getAccountDetails(u.store, sesh)
	if err != nil {
		return nil, fmt.Errorf("account details: %w", err)
	}

	return response.NewTemplate(
		[]string{"blog_repository_flow.html"},
		util.PageInfo{
			Data: struct {
				Title          string
				UserInfo       *session.UserInfo
				AccountDetails AccountDetails
				ServiceName    string
				Repositories   []Repository
				Themes         []string
			}{
				Title:          "Repository Flow",
				UserInfo:       session.ConvertSessionToUserInfo(sesh),
				AccountDetails: details,
				ServiceName:    config.Config.Progstack.ServiceName,
				Repositories:   buildRepositoriesInfo(repos),
				Themes:         blog.BuildThemes(config.Config.ProgstackSsg.Themes),
			},
		},
		template.FuncMap{},
		logger,
	), nil
}

func buildRepositoriesInfo(repos []model.Repository) []Repository {
	var res []Repository
	for _, repo := range repos {
		res = append(res, Repository{
			Value: repo.RepositoryID,
			Name:  repo.FullName,
		})
	}
	return res
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
	logger := r.Logger()
	logger.Println("Account handler...")

	r.MixpanelTrack("Account")

	sesh := r.Session()
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
		template.FuncMap{},
		logger,
	), nil
}

func getStorageDetails(s *model.Store, session *session.Session) (StorageDetails, error) {
	/* calculate storage */
	userBytes, err := authz.UserStorageUsed(s, session.GetUserID())
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

func getAccountDetails(s *model.Store, session *session.Session) (AccountDetails, error) {
	/* get github info */
	accountDetails := AccountDetails{
		Username:        session.GetUsername(),
		Email:           session.GetEmail(),
		IsLinked:        false,
		HasInstallation: false,
		GithubEmail:     "",
	}
	linked := true
	ghAccount, err := s.GetGithubAccountByUserID(
		context.TODO(), session.GetUserID(),
	)
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
		context.TODO(), session.GetUserID(),
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
	sub, err := s.GetStripeSubscriptionByUserID(
		context.TODO(), session.GetUserID(),
	)
	if err != nil {
		return AccountDetails{}, fmt.Errorf(
			"error getting stripe subscription details: %w",
			err,
		)
	}
	log.Printf("subName: %s modelName: %s", sub.SubName, model.SubNameScout)

	accountDetails.Subscription = Subscription{
		Plan: string(sub.SubName),
	}
	return accountDetails, nil
}
