package user

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/billing"
	"github.com/xr0-org/progstack/internal/blog"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/installation"
	"github.com/xr0-org/progstack/internal/model"
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
		session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if !ok {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}

		/* get account details */
		details, err := getAccountDetails(u.store, session)
		if err != nil {
			log.Printf("error getting acount details: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		blogs, err := blog.GetBlogsInfo(u.store, session.UserID)
		if err != nil {
			log.Println("error getting blogs for user `%d': %v", session.UserID, err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		githubInstallAppUrl := fmt.Sprintf(installation.GhInstallUrlTemplate, config.Config.Github.AppName)
		util.ExecTemplate(w, []string{"home.html", "blogs.html"},
			util.PageInfo{
				Data: struct {
					Title               string
					Session             *auth.Session
					GithubInstallAppUrl string
					AccountDetails      AccountDetails
					Blogs               []blog.BlogInfo
				}{
					Title:               "Home",
					Session:             session,
					GithubInstallAppUrl: githubInstallAppUrl,
					AccountDetails:      details,
					Blogs:               blogs,
				},
			},
			template.FuncMap{},
		)
	}
}

type Repository struct {
	Value int64
	Name  string
}

func (u *UserService) CreateNewBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("create new blog handler...")

		session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if !ok {
			log.Println("no user found")
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		hasInstallation, err := u.store.InstallationExistsForUserID(context.TODO(), session.UserID)
		if err != nil {
			log.Printf("error getting installation for user: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		repos, err := u.store.ListOrderedRepositoriesByUserID(context.TODO(), session.UserID)
		if err != nil {
			if err != sql.ErrNoRows {
				log.Printf("error getting repositories: %v", err)
				http.Error(w, "", http.StatusInternalServerError)
				return
			}
		}

		details, err := getAccountDetails(u.store, session)
		if err != nil {
			log.Printf("error getting acount details: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		githubInstallAppUrl := fmt.Sprintf(installation.GhInstallUrlTemplate, config.Config.Github.AppName)
		util.ExecTemplate(w, []string{"blog_create.html"},
			util.PageInfo{
				Data: struct {
					Title               string
					Session             *auth.Session
					AccountDetails      AccountDetails
					GithubInstallAppUrl string
					HasInstallation     bool
					ServiceName         string
					Repositories        []Repository
				}{
					Title:               "Create New Blog",
					Session:             session,
					AccountDetails:      details,
					GithubInstallAppUrl: githubInstallAppUrl,
					HasInstallation:     hasInstallation,
					ServiceName:         config.Config.Progstack.ServiceName,
					Repositories:        buildRepositoriesInfo(repos),
				},
			},
			template.FuncMap{},
		)
	}
}

func buildRepositoriesInfo(repos []model.Repository) []Repository {
	res := make([]Repository, len(repos))
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

		session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if !ok {
			log.Printf("user auth session not found\n")
			http.Error(w, "", http.StatusUnauthorized)
			return
		}

		details, err := getAccountDetails(u.store, session)
		if err != nil {
			log.Printf("error getting acount details: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		util.ExecTemplate(w, []string{"account.html"},
			util.PageInfo{
				Data: struct {
					Title          string
					Session        *auth.Session
					AccountDetails AccountDetails
				}{
					Title:          "Home",
					Session:        session,
					AccountDetails: details,
				},
			},
			template.FuncMap{},
		)
	}
}

func getAccountDetails(s *model.Store, session *auth.Session) (AccountDetails, error) {
	/* get github info */
	accountDetails := AccountDetails{
		Username:        session.Username,
		Email:           session.Email,
		IsLinked:        false,
		HasInstallation: false,
		GithubEmail:     "",
	}
	linked := true
	ghAccount, err := s.GetGithubAccountByUserID(context.TODO(), session.UserID)
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

	hasInstallation, err := s.InstallationExistsForUserID(context.TODO(), session.UserID)
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
	sub, err := s.GetStripeSubscriptionByUserID(context.TODO(), session.UserID)
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

		session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if !ok {
			log.Printf("user auth session not found\n")
			http.Error(w, "", http.StatusUnauthorized)
			return
		}

		/* XXX: need to call stripe to stop billing */

		if err := u.store.DeleteUserByUserID(context.TODO(), session.UserID); err != nil {
			log.Printf("error deleting user: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
