package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/xr0-org/progstack/internal/analytics"
	"github.com/xr0-org/progstack/internal/authz"
	"github.com/xr0-org/progstack/internal/blog"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/installation"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

type UserService struct {
	store    *model.Store
	mixpanel *analytics.MixpanelClientWrapper
}

func NewUserService(
	s *model.Store, m *analytics.MixpanelClientWrapper,
) *UserService {
	return &UserService{
		store:    s,
		mixpanel: m,
	}
}

func (u *UserService) Home() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("Home handler...")

		u.mixpanel.Track("Home", r)

		/* get session */
		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
			http.Error(w, "", http.StatusNotFound)
			return
		}

		blogs, err := blog.GetBlogsInfo(u.store, sesh.GetUserID())
		if err != nil {
			logger.Printf("Error getting blogs for user `%d': %v\n", sesh.GetUserID(), err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		githubInstallAppUrl := fmt.Sprintf(installation.GhInstallUrlTemplate, config.Config.Github.AppName)
		util.ExecTemplate(w, []string{"home.html", "blogs.html"},
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
		)
	}
}

func (u *UserService) CreateNewBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("CreateNewBlog handler...")

		u.mixpanel.Track("CreateNewBlog", r)

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
			http.Error(w, "", http.StatusNotFound)
			return
		}

		can, err := authz.CanCreateSite(u.store, sesh)
		if err != nil {
			logger.Printf("Error performing action: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		if !can {
			logger.Printf("User not authorized")
			http.Error(w, "", http.StatusForbidden)
			return
		}

		util.ExecTemplate(w, []string{"blog_create.html"},
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
		)
	}
}

func (u *UserService) FolderFlow() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Printf("FolderFlow handler...")

		u.mixpanel.Track("FolderFlow", r)

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
			http.Error(w, "", http.StatusNotFound)
			return
		}

		util.ExecTemplate(w, []string{"blog_folder_flow.html"},
			util.PageInfo{
				Data: struct {
					Title       string
					UserInfo    *session.UserInfo
					ServiceName string
					Themes      []string
				}{
					Title:       "Folder Flow",
					UserInfo:    session.ConvertSessionToUserInfo(sesh),
					ServiceName: config.Config.Progstack.ServiceName,
					Themes:      blog.BuildThemes(config.Config.ProgstackSsg.Themes),
				},
			},
			template.FuncMap{},
			logger,
		)
	}
}

func (u *UserService) GithubInstallation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Printf("GithubInstallation handler...")

		u.mixpanel.Track("GithubInstallation", r)

		if err := u.githubInstallation(w, r); err != nil {
			var customErr *util.CustomError
			if errors.As(err, &customErr) {
				logger.Printf("Custom Error: %v\n", customErr)
				http.Error(
					w, customErr.Error(), customErr.Code,
				)
			} else {
				logger.Printf("Internal Server Error: %v\n", err)
				http.Error(
					w,
					"Internal Server Error",
					http.StatusInternalServerError,
				)
			}
			return
		}
	}
}

func (u *UserService) githubInstallation(w http.ResponseWriter, r *http.Request) error {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		return util.CreateCustomError(
			"user not found",
			http.StatusNotFound,
		)
	}

	if err := u.store.UpdateAwaitingGithubUpdate(
		context.TODO(),
		model.UpdateAwaitingGithubUpdateParams{
			ID:               sesh.GetUserID(),
			GhAwaitingUpdate: true,
		},
	); err != nil {
		return fmt.Errorf("error updating awaiting: %w", err)
	}

	http.Redirect(
		w,
		r,
		fmt.Sprintf(
			installation.GhInstallUrlTemplate,
			config.Config.Github.AppName,
		),
		http.StatusTemporaryRedirect,
	)
	return nil
}

func (u *UserService) awaitupdate(r *http.Request) error {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		return util.CreateCustomError(
			"user not found",
			http.StatusNotFound,
		)
	}
	id := sesh.GetUserID()

	/* TODO: get from config */
	var (
		timeout = 5 * time.Second
		step    = 100 * time.Millisecond
	)
	for until := time.Now().Add(timeout); time.Now().Before(until); time.Sleep(step) {
		awaiting, err := u.store.IsAwaitingGithubUpdate(
			context.TODO(), id,
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
			ID:               id,
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

func (u *UserService) RepositoryFlow() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("RepositoryFlow handler...")

		u.mixpanel.Track("RepositoryFlow", r)

		if err := u.repositoryFlow(w, r); err != nil {
			var customErr *util.CustomError
			if errors.As(err, &customErr) {
				logger.Printf("Custom Error: %v\n", customErr)
				http.Error(
					w, customErr.Error(), customErr.Code,
				)
			} else {
				logger.Printf("Internal Server Error: %v\n", err)
				http.Error(
					w,
					"Internal Server Error",
					http.StatusInternalServerError,
				)
			}
			return
		}
	}
}

func (u *UserService) repositoryFlow(w http.ResponseWriter, r *http.Request) error {
	logger := logging.Logger(r)

	if err := u.awaitupdate(r); err != nil {
		return fmt.Errorf("error awaiting update: %w", err)
	}

	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		return util.CreateCustomError(
			"user not found", http.StatusNotFound,
		)
	}

	repos, err := u.store.ListOrderedRepositoriesByUserID(
		context.TODO(), sesh.GetUserID(),
	)
	if err != nil {
		return fmt.Errorf("error getting repositories: %w", err)
	}

	details, err := getAccountDetails(u.store, sesh)
	if err != nil {
		return fmt.Errorf("error getting acount details: %w", err)
	}

	util.ExecTemplate(w, []string{"blog_repository_flow.html"},
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
	)
	return nil
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

func (u *UserService) Account() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("Account handler...")

		u.mixpanel.Track("Account", r)

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No auth session")
			http.Error(w, "", http.StatusUnauthorized)
			return
		}

		accountDetails, err := getAccountDetails(u.store, sesh)
		if err != nil {
			logger.Printf("Error getting account details: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		storageDetails, err := getStorageDetails(u.store, sesh)
		if err != nil {
			logger.Printf("Error getting storage details: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		util.ExecTemplate(w, []string{"account.html"},
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
		)
	}
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
