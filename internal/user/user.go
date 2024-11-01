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

	"github.com/xr0-org/progstack/internal/billing"
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
	return &UserService{
		store: s,
	}
}

func (u *UserService) Home() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("home handler...")
		/* XXX: add metrics */

		/* get session */
		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}

		/* get account details */
		details, err := getAccountDetails(u.store, sesh)
		if err != nil {
			log.Printf("error getting acount details: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		blogs, err := blog.GetBlogsInfo(u.store, sesh.GetUserID())
		if err != nil {
			log.Printf("error getting blogs for user `%d': %v\n", sesh.GetUserID(), err)
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
					AccountDetails      AccountDetails
					Blogs               []blog.BlogInfo
				}{
					Title:               "Home",
					UserInfo:            session.ConvertSessionToUserInfo(sesh),
					GithubInstallAppUrl: githubInstallAppUrl,
					AccountDetails:      details,
					Blogs:               blogs,
				},
			},
			template.FuncMap{},
		)
	}
}

func (u *UserService) CreateNewBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("respository flow handler...")

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			log.Println("no user found")
			http.Error(w, "", http.StatusNotFound)
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
		)
	}
}

func (u *UserService) FolderFlow() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("folder flow handler...")

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			log.Println("no user found")
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
		)
	}
}

func (u *UserService) GithubInstallation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := u.githubInstallation(w, r); err != nil {
			var customErr *util.CustomError
			if errors.As(err, &customErr) {
				http.Error(
					w, customErr.Error(), customErr.Code,
				)
			} else {
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
		if err := u.repositoryFlow(w, r); err != nil {
			var customErr *util.CustomError
			if errors.As(err, &customErr) {
				http.Error(
					w, customErr.Error(), customErr.Code,
				)
			} else {
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
	if err := u.awaitupdate(r); err != nil {
		return fmt.Errorf("error awaiting update: %w", err)
	}

	log.Println("create new blog handler...")

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

type Subscription struct {
	IsSubscribed       bool
	Plan               string
	CurrentPeriodStart string
	CurrentPeriodEnd   string
	Amount             string
}

func (u *UserService) Account() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("account handler...")

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			log.Printf("user auth session not found\n")
			http.Error(w, "", http.StatusUnauthorized)
			return
		}

		details, err := getAccountDetails(u.store, sesh)
		if err != nil {
			log.Printf("error getting acount details: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		util.ExecTemplate(w, []string{"account.html"},
			util.PageInfo{
				Data: struct {
					Title          string
					UserInfo       *session.UserInfo
					AccountDetails AccountDetails
				}{
					Title:          "Home",
					UserInfo:       session.ConvertSessionToUserInfo(sesh),
					AccountDetails: details,
				},
			},
			template.FuncMap{},
		)
	}
}

func getAccountDetails(s *model.Store, session *session.Session) (AccountDetails, error) {
	/* calculate storage */
	userBytes, err := UserBytes(s, session.GetUserID())
	if err != nil {
		return AccountDetails{}, err
	}
	userMegaBytes := float64(userBytes) / (1024 * 1024)

	/* get github info */
	accountDetails := AccountDetails{
		Username:        session.GetUsername(),
		Email:           session.GetEmail(),
		IsLinked:        false,
		HasInstallation: false,
		GithubEmail:     "",
		StorageUsed:     fmt.Sprintf("%.2f", userMegaBytes),
		StorageLimit:    "10",
	}
	linked := true
	ghAccount, err := s.GetGithubAccountByUserID(context.TODO(), session.GetUserID())
	if err != nil {
		if err != sql.ErrNoRows {
			return AccountDetails{}, fmt.Errorf("error getting account details: %w", err)
		}
		/* no linked Github account*/
		linked = false
	}
	if linked {
		accountDetails.IsLinked = true
		accountDetails.GithubEmail = ghAccount.GhEmail
	}

	hasInstallation, err := s.InstallationExistsForUserID(context.TODO(), session.GetUserID())
	if err != nil {
		return AccountDetails{}, fmt.Errorf("error checking if user has installation: %w", err)
	}
	if hasInstallation {
		accountDetails.HasInstallation = true
	}

	/* get stripe subscription */
	subscription := Subscription{
		IsSubscribed: false,
	}
	subscribed := true
	sub, err := s.GetStripeSubscriptionByUserID(context.TODO(), session.GetUserID())
	if err != nil {
		if err != sql.ErrNoRows {
			return AccountDetails{}, fmt.Errorf("error getting stripe subscription details: %w", err)
		}
		/* no sub */
		subscribed = false
	}

	/* get price info from stripe */
	if subscribed {
		subscription.IsSubscribed = true
		subscription.Plan = "basic" /* XXX: fix */
		subscription.CurrentPeriodStart = sub.CurrentPeriodStart.Format("Jan 02 2006 03:04PM")
		subscription.CurrentPeriodEnd = sub.CurrentPeriodEnd.Format("Jan 02 2006 03:04PM")
		subscription.Amount = billing.ConvertCentsToDollars(sub.Amount)
	}
	accountDetails.Subscription = subscription
	return accountDetails, nil
}

func (u *UserService) Delete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("user delete handler...")

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			log.Printf("user auth session not found\n")
			http.Error(w, "", http.StatusUnauthorized)
			return
		}

		/* XXX: need to call stripe to stop billing */

		if err := u.store.DeleteUserByUserID(context.TODO(), sesh.GetUserID()); err != nil {
			log.Printf("error deleting user: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
