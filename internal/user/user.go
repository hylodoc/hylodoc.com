package user

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/billing"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/util"
)

const (
	ghInstallUrlTemplate = "https://github.com/apps/%s/installations/new"
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
		details, err := u.accountDetails(w, r, session)
		if err != nil {
			log.Printf("error getting acount details: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		blogs, err := getBlogsInfo(u.store, session.UserID)
		if err != nil {
			log.Printf("could not get Installations info for user `%d': %v", session.UserID, err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		githubInstallAppUrl := fmt.Sprintf(ghInstallUrlTemplate, config.Config.Github.AppName)
		util.ExecTemplate(w, []string{"home.html", "blogs.html"},
			util.PageInfo{
				Data: struct {
					Title               string
					Session             *auth.Session
					GithubInstallAppUrl string
					AccountDetails      AccountDetails
					Blogs               []BlogInfo
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

type BlogInfo struct {
	ID             int32
	Domain         string
	DomainUrl      string
	RepositoryUrl  string
	SubscribersUrl string
	MetricsUrl     string
	Type           string
	Status         string
	UpdatedAt      time.Time
	IsRepository   bool
	IsLive         bool
}

func getBlogsInfo(s *model.Store, userID int32) ([]BlogInfo, error) {
	blogs, err := s.ListBlogsByUserID(context.TODO(), userID)
	if err != nil {
		/* should not be possible to have an installation with no repositories */
		return []BlogInfo{}, err
	}
	var info []BlogInfo
	for _, blog := range blogs {
		subdomain := blog.DemoSubdomain
		if blog.Subdomain.Valid {
			subdomain = blog.Subdomain.String
		}
		isRepository := false
		if blog.BlogType == model.BlogTypeRepository {
			isRepository = true
		}
		isLive := false
		if blog.Status == model.BlogStatusLive {
			isLive = true
		}
		blogInfo := BlogInfo{
			ID:             blog.ID,
			Domain:         buildDomain(subdomain),
			DomainUrl:      buildDomainUrl(subdomain),
			RepositoryUrl:  blog.GhUrl,
			SubscribersUrl: buildSubscribersUrl(blog.ID),
			MetricsUrl:     buildMetricsUrl(blog.ID),
			Type:           string(blog.BlogType),
			Status:         string(blog.Status),
			UpdatedAt:      blog.UpdatedAt,
			IsRepository:   isRepository,
			IsLive:         isLive,
		}
		info = append(info, blogInfo)
	}
	return info, nil
}

func buildDomain(subdomain string) string {
	return fmt.Sprintf(
		"%s.%s",
		subdomain,
		config.Config.Progstack.ServiceName,
	)
}

func buildDomainUrl(subdomain string) string {
	return fmt.Sprintf(
		"%s://%s.%s",
		config.Config.Progstack.Protocol,
		subdomain,
		config.Config.Progstack.ServiceName,
	)
}

func buildSubscribersUrl(blogID int32) string {
	return fmt.Sprintf(
		"%s://%s/user/blogs/%d/subscribers",
		config.Config.Progstack.Protocol,
		config.Config.Progstack.ServiceName,
		blogID,
	)
}

func buildMetricsUrl(blogID int32) string {
	return fmt.Sprintf(
		"%s://%s/user/blogs/%d/metrics",
		config.Config.Progstack.Protocol,
		config.Config.Progstack.ServiceName,
		blogID,
	)
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

		details, err := u.accountDetails(w, r, session)
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

func (u *UserService) accountDetails(w http.ResponseWriter, r *http.Request, session *auth.Session) (AccountDetails, error) {
	/* get github info */
	accountDetails := AccountDetails{
		Username:        session.Username,
		Email:           session.Email,
		IsLinked:        false,
		HasInstallation: false,
		GithubEmail:     "",
	}
	linked := true
	ghAccount, err := u.store.GetGithubAccountByUserID(context.TODO(), session.UserID)
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

	hasInstallation, err := u.store.InstallationExistsForUserID(context.TODO(), session.UserID)
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
	sub, err := u.store.GetStripeSubscriptionByUserID(context.TODO(), session.UserID)
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
