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

func (u *UserService) HomeCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("homecallback handler...")

		/* get session */
		session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if !ok {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}

		start := time.Now()
		for {
			duration := time.Since(start)
			log.Printf("duration: %s\n", duration.String())
			if duration > 10*time.Second {
				log.Printf("homecallback handler timed out for user `%s'\n", session.UserID)
				http.Error(w, "callback timed out", http.StatusRequestTimeout)
				return
			}

			/* get installation info */
			_, err := getInstallationsInfo(u.store, session.UserID)
			if err != nil {
				if err != sql.ErrNoRows {
					log.Printf("error fetching installations info for user `%d': %v\n", session.UserID, err)
					http.Error(w, "", http.StatusInternalServerError)
					return
				}
				/* not found, wait for event to be processed */
				time.Sleep(2 * time.Second)
				continue
			}
			/* got installation info */
			break
		}
		/* found installation info */
		http.Redirect(w, r, "/user/home", http.StatusSeeOther)
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

		installationsInfo, err := getInstallationsInfo(u.store, session.UserID)
		if err != nil {
			log.Printf("could not get Installations info for user `%d': %v", session.UserID, err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		ghInstallUrl := fmt.Sprintf(ghInstallUrlTemplate, config.Config.Github.AppName)
		util.ExecTemplate(w, []string{"home.html", "installations.html", "blogs.html"},
			util.PageInfo{
				Data: struct {
					Title               string
					Session             *auth.Session
					GithubAppInstallUrl string
					Installations       []InstallationInfo
				}{
					Title:               "Home",
					Session:             session,
					GithubAppInstallUrl: ghInstallUrl,
					Installations:       installationsInfo,
				},
			},
			template.FuncMap{},
		)
	}
}

type InstallationInfo struct {
	GithubID  int64      `json:"github_id"`
	CreatedAt time.Time  `json:"created_at"`
	Blogs     []BlogInfo `json:"blogs"`
}

type BlogInfo struct {
	ID         int32     `json:"ID"`
	Name       string    `json:"name"`
	GithubUrl  string    `json:"html_url"`
	WebsiteUrl string    `json:"website_url"`
	Subdomain  string    `json:"subdomain"`
	Active     bool      `json:"active"`
	CreatedAt  time.Time `json:"created_at"`
}

func getInstallationsInfo(s *model.Store, userID int32) ([]InstallationInfo, error) {
	/* get installations for user */
	installations, err := s.ListInstallationsForUser(context.TODO(), userID)
	if err != nil {
		return []InstallationInfo{}, err
	}
	/* populate the installation info get repositories */
	var info []InstallationInfo
	for _, dbInstallation := range installations {
		blogsInfo, err := getBlogsInfo(s, dbInstallation.GhInstallationID)
		if err != nil {
			return []InstallationInfo{}, fmt.Errorf("error getting RepositoriesInfo: %w", err)
		}
		installationInfo := InstallationInfo{
			GithubID:  dbInstallation.GhInstallationID,
			CreatedAt: dbInstallation.CreatedAt,
			Blogs:     blogsInfo,
		}
		info = append(info, installationInfo)
	}
	return info, nil
}

func getBlogsInfo(s *model.Store, ghInstallationID int64) ([]BlogInfo, error) {
	blogs, err := s.ListBlogsForInstallationByGhInstallationID(context.TODO(), ghInstallationID)
	if err != nil {
		/* should not be possible to have an installation with no repositories */
		return []BlogInfo{}, err
	}
	var info []BlogInfo
	for _, blog := range blogs {
		subdomain := "Not configured"
		if blog.Subdomain.Valid {
			subdomain = blog.Subdomain.String
		}
		blogInfo := BlogInfo{
			ID:         blog.ID,
			Name:       blog.GhName,
			GithubUrl:  blog.GhUrl,
			WebsiteUrl: fmt.Sprintf("http://%s.localhost:7999", blog.DemoSubdomain), /* XXX: how to accurately track this since website is generated from repos */
			Subdomain:  subdomain,
			Active:     blog.Active,
			CreatedAt:  blog.CreatedAt,
		}
		info = append(info, blogInfo)
	}
	return info, nil
}

type AccountDetails struct {
	Username     string
	Email        string
	IsLinked     bool
	GithubEmail  string
	Subscription Subscription
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
		Username:    session.Username,
		Email:       session.Email,
		IsLinked:    false,
		GithubEmail: "",
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
		subscription.Amount = billing.ConvertCentsToDollars(sub.Amount) /* XXX: fix */
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
