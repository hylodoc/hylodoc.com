package blog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack-ssg/pkg/ssg"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

type BlogService struct {
	store        *model.Store
	resendClient *resend.Client
}

func NewBlogService(store *model.Store, resendClient *resend.Client) *BlogService {
	return &BlogService{store: store, resendClient: resendClient}
}

/* Blog configuration page */
func (b *BlogService) Config() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("blog config handler...")

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}
		blogID := mux.Vars(r)["blogID"]
		intBlogID, err := strconv.ParseInt(blogID, 10, 32)
		if err != nil {
			log.Println("error converting string path var to blogID: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		blogInfo, err := getBlogInfo(b.store, int32(intBlogID))
		if err != nil {
			log.Println("error getting blog for user `%d': %v", sesh.GetUserID(), err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		util.ExecTemplate(w, []string{"blog_config.html"},
			util.PageInfo{
				Data: struct {
					Title        string
					UserInfo     *session.UserInfo
					ID           int32
					Blog         BlogInfo
					Themes       []string
					CurrentTheme string
				}{
					Title:        "Blog Setup",
					UserInfo:     session.ConvertSessionToUserInfo(sesh),
					ID:           int32(intBlogID),
					Blog:         blogInfo,
					Themes:       BuildThemes(config.Config.ProgstackSsg.Themes),
					CurrentTheme: string(blogInfo.Theme),
				},
			},
			template.FuncMap{},
		)
	}
}

func BuildThemes(themes map[string]config.Theme) []string {
	var keys []string
	for key := range themes {
		keys = append(keys, key)
	}
	return keys
}

func ConvertCentsToDollars(cents int64) string {
	dollars := float64(cents) / 100.0
	return fmt.Sprintf("$%.2f", dollars)
}

/* Launch Blog */

type LaunchUserBlogParams struct {
	RepositoryPath string
	Subdomain      string
	Theme          string
}

func launchUserBlog(params LaunchUserBlogParams) error {
	if _, err := os.Stat(params.RepositoryPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("repository at `%s' does not exist on disk: %w", params.RepositoryPath, err)
		}
		return err
	}
	site := filepath.Join(
		config.Config.Progstack.WebsitesPath,
		params.Subdomain,
	)
	if err := ssg.GenerateSite(params.RepositoryPath, site, config.Config.ProgstackSsg.Themes[params.Theme].Path); err != nil {
		return fmt.Errorf("error generating site: %w", err)
	}
	return nil
}

/* Theme */

func (b *BlogService) ThemeSubmit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("theme submit handler...")

		if err := b.themeSubmit(w, r); err != nil {
			log.Printf("error updating theme: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			response := map[string]string{"message": "An unexpected error occured"}
			json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusOK)
		response := map[string]string{"message": "Theme changed successsfully!"}
		json.NewEncoder(w).Encode(response)
	}
}

type ThemeRequest struct {
	Theme string `json:"theme"`
}

func (b *BlogService) themeSubmit(w http.ResponseWriter, r *http.Request) error {
	blogID := mux.Vars(r)["blogID"]
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return err
	}

	var req ThemeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return fmt.Errorf("error decoding body: %w", err)
	}

	theme, err := validateTheme(req.Theme)
	if err != nil {
		return err
	}

	if err := b.store.SetBlogThemeByID(context.TODO(), model.SetBlogThemeByIDParams{
		ID:    int32(intBlogID),
		Theme: theme,
	}); err != nil {
		return fmt.Errorf("error setting blog theme: %w", err)
	}

	return nil
}

/* Git branch info */

func (b *BlogService) TestBranchSubmit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("test branch submit handler...")

		if err := b.testBranchSubmit(w, r); err != nil {
			log.Printf("error updating test branch: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			response := map[string]string{"message": "An unexpected error occurred"}
			json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusOK)
		response := map[string]string{"message": "Test branch submitted successsfully!"}
		json.NewEncoder(w).Encode(response)
	}
}

type TestBranchRequest struct {
	Branch string `json:"branch"`
}

func (b *BlogService) testBranchSubmit(w http.ResponseWriter, r *http.Request) error {
	blogID := mux.Vars(r)["blogID"]
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return err
	}

	var req TestBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return fmt.Errorf("error decoding body: %w", err)
	}
	log.Printf("test branch: %s\n", req.Branch)

	/* XXX: validate input before writing to db */
	if err := b.store.SetTestBranchByID(context.TODO(), model.SetTestBranchByIDParams{
		ID: int32(intBlogID),
		TestBranch: sql.NullString{
			Valid:  true,
			String: req.Branch,
		},
	}); err != nil {
		return fmt.Errorf("error updating branch info: %w", err)
	}
	return nil
}

func (b *BlogService) LiveBranchSubmit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("live branch submit handler...")

		if err := b.liveBranchSubmit(w, r); err != nil {
			/* XXX: custom error handling of codes */
			log.Printf("error updating live branch: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			response := map[string]string{"message": "An unexpected error occurred"}
			json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusOK)
		response := map[string]string{"message": "Live branch submitted successsfully!"}
		json.NewEncoder(w).Encode(response)
	}
}

type LiveBranchRequest struct {
	Branch string `json:"branch"`
}

func (b *BlogService) liveBranchSubmit(w http.ResponseWriter, r *http.Request) error {
	blogID := mux.Vars(r)["blogID"]
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return err
	}

	/* submitted with form and no javascript */
	var req LiveBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return fmt.Errorf("error decoding body: %w", err)
	}
	log.Printf("live branch: %s\n", req.Branch)

	/* XXX: validate input before wrinting to db */
	err = b.store.SetLiveBranchByID(context.TODO(), model.SetLiveBranchByIDParams{
		ID: int32(intBlogID),
		LiveBranch: sql.NullString{
			Valid:  true,
			String: req.Branch,
		},
	})
	if err != nil {
		return fmt.Errorf("error updating branch info: %w", err)
	}
	return nil
}

type SetStatusSubmitResponse struct {
	Message string `json:"message"`
}

func (b *BlogService) SetStatusSubmit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("set status handler...")

		w.Header().Set("Content-Type", "application/json")

		change, err := b.setStatusSubmit(w, r)
		if err != nil {
			userErr, ok := err.(util.UserError)
			if ok {
				log.Printf("Client Error: %v\n", userErr)
				w.WriteHeader(http.StatusBadRequest)
				if err := json.NewEncoder(w).Encode(util.ErrorResponse{
					Message: userErr.Error(),
				}); err != nil {
					log.Printf("Failed to encode response: %v\n", err)
					http.Error(w, "", http.StatusInternalServerError)
					return
				}
				return
			}
			log.Printf("Internal Server Error: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		message := ""
		if change.IsLive {
			message = fmt.Sprintf("%s is live!", change.Domain)
		} else {
			message = fmt.Sprintf("%s was taken offline successfully.", change.Domain)
		}
		if err = json.NewEncoder(w).Encode(SetStatusSubmitResponse{
			Message: message,
		}); err != nil {
			log.Printf("Failed to encode response: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}
}

type SetStatusRequest struct {
	Status string `json:"status"`
}

func (b *BlogService) setStatusSubmit(w http.ResponseWriter, r *http.Request) (statusChangeResponse, error) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		return statusChangeResponse{}, fmt.Errorf("no user found")
	}
	blogID := mux.Vars(r)["blogID"]
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return statusChangeResponse{}, err
	}

	var req SetStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return statusChangeResponse{}, fmt.Errorf("error decoding body: %w", err)
	}

	change, err := handleStatusChange(int32(intBlogID), req.Status, sesh.GetEmail(), b.store)
	if err != nil {
		return statusChangeResponse{}, err
	}
	return change, nil
}

type statusChangeResponse struct {
	Domain string
	IsLive bool
}

func handleStatusChange(blogID int32, status, email string, s *model.Store) (statusChangeResponse, error) {
	blog, err := s.GetBlogByID(context.TODO(), blogID)
	if err != nil {
		return statusChangeResponse{}, fmt.Errorf("error getting blog `%d': %w", blogID, err)
	}

	if err := validateStatusChange(status, string(blog.Status)); err != nil {
		return statusChangeResponse{}, err
	}

	switch status {
	case "live":
		return setBlogToLive(blog, s)
	case "offline":
		return setBlogToOffline(blog, s)
	default:
		return statusChangeResponse{}, fmt.Errorf("invalid status: %s", status)
	}
}

func validateStatusChange(request, current string) error {
	if request != string(model.BlogStatusLive) && request != string(model.BlogStatusOffline) {
		return fmt.Errorf(
			"request needs to either be `%s' or `%s'",
			model.BlogStatusLive,
			model.BlogStatusOffline,
		)
	}
	if request == current {
		return util.UserError{
			Message: fmt.Sprintf("state is already %s", current),
			Code:    http.StatusBadRequest,
		}
	}
	return nil
}

func setBlogToLive(blog model.Blog, s *model.Store) (statusChangeResponse, error) {
	fmt.Printf("repo disk path: %s\n", blog.RepositoryPath)

	if err := launchUserBlog(LaunchUserBlogParams{
		RepositoryPath: blog.RepositoryPath,
		Subdomain:      blog.Subdomain,
		Theme:          string(blog.Theme),
	}); err != nil {
		return statusChangeResponse{}, fmt.Errorf("error launching blog `%d': %w", blog.ID, err)
	}

	/* update status to live */
	if err := s.SetBlogStatusByID(context.TODO(), model.SetBlogStatusByIDParams{
		ID:     blog.ID,
		Status: model.BlogStatusLive,
	}); err != nil {
		return statusChangeResponse{}, fmt.Errorf("error setting status to %s: %w", model.BlogStatusLive, err)
	}
	return statusChangeResponse{
		Domain: blog.Subdomain,
		IsLive: true,
	}, nil
}

func setBlogToOffline(blog model.Blog, s *model.Store) (statusChangeResponse, error) {
	site := filepath.Join(
		config.Config.Progstack.WebsitesPath,
		blog.Subdomain,
	)
	if err := os.RemoveAll(site); err != nil {
		return statusChangeResponse{}, fmt.Errorf("error deleting website `%s' from disk: %w", blog.Subdomain, err)
	}
	err := s.SetBlogStatusByID(context.TODO(), model.SetBlogStatusByIDParams{
		ID:     blog.ID,
		Status: model.BlogStatusOffline,
	})
	if err != nil {
		return statusChangeResponse{}, fmt.Errorf("error setting status to `%s': %w", model.BlogStatusOffline, err)
	}
	return statusChangeResponse{
		Domain: blog.Subdomain,
		IsLive: false,
	}, nil
}
