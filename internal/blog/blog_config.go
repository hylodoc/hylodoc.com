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
	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
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

		session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
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

		blog, err := getBlogInfo(b.store, int32(intBlogID))
		if err != nil {
			log.Println("error getting blog for user `%d': %v", session.UserID, err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		util.ExecTemplate(w, []string{"blog_config.html"},
			util.PageInfo{
				Data: struct {
					Title   string
					Session *auth.Session
					ID      int32
					Blog    BlogInfo
				}{
					Title:   "Blog Setup",
					Session: session,
					ID:      int32(intBlogID),
					Blog:    blog,
				},
			},
			template.FuncMap{},
		)
	}
}

func ConvertCentsToDollars(cents int64) string {
	dollars := float64(cents) / 100.0
	return fmt.Sprintf("$%.2f", dollars)
}

/* Launch Blog */

type LaunchBlogParams struct {
	GhRepoFullName string
	Subdomain      string
}

type LaunchUserBlogParams struct {
	GhRepoFullName string
	Subdomain      string
}

func LaunchUserBlog(params LaunchUserBlogParams) error {
	repo := filepath.Join(
		config.Config.Progstack.RepositoriesPath,
		params.GhRepoFullName,
	)
	if _, err := os.Stat(repo); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("repository does not exist on disk: %w", err)
		}
		return err
	}
	site := filepath.Join(
		config.Config.Progstack.WebsitesPath,
		params.Subdomain,
	)
	if err := ssg.GenerateSite(repo, site, "progstack-ssg/theme/lit"); err != nil {
		return fmt.Errorf("error generating site: %w", err)
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
	err = b.store.SetTestBranchByID(context.TODO(), model.SetTestBranchByIDParams{
		ID: int32(intBlogID),
		TestBranch: sql.NullString{
			Valid:  true,
			String: req.Branch,
		},
	})
	if err != nil {
		return fmt.Errorf("error updating branch info: %w", err)
	}
	return nil
}

func (b *BlogService) LiveBranchSubmit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("live branch submit handler...")

		if err := b.liveBranchSubmit(w, r); err != nil {
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

func (b *BlogService) SetStatusSubmit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("set status handler...")

		change, err := b.setStatusSubmit(w, r)
		if err != nil {
			log.Printf("error setting status: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			response := map[string]string{"message": "An unexpected error occurred"}
			json.NewEncoder(w).Encode(response)
		}
		w.WriteHeader(http.StatusOK)
		var response map[string]string
		if change.IsLive {
			response = map[string]string{"message": fmt.Sprintf("%s is live!", change.Domain)}
		} else {
			response = map[string]string{"message": fmt.Sprintf("%s was taken offline successfully.", change.Domain)}
		}
		json.NewEncoder(w).Encode(response)
	}
}

type SetStatusRequest struct {
	Status string `json:"status"`
}

func (b *BlogService) setStatusSubmit(w http.ResponseWriter, r *http.Request) (statusChangeResponse, error) {
	session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
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

	change, err := handleStatusChange(int32(intBlogID), req.Status, session.Email, b.store)
	if err != nil {
		return statusChangeResponse{}, fmt.Errorf("error handling status change: %w", err)
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
		return statusChangeResponse{}, util.UserError{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("error: %w", err),
		}
	}

	switch status {
	case "live":
		return statusChangeResponse{
			Domain: blog.Subdomain,
			IsLive: true,
		}, launchBlog(blog, email, s)
	case "offline":
		return statusChangeResponse{
			Domain: blog.Subdomain,
			IsLive: false,
		}, deleteBlog(blog, s)
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
		return fmt.Errorf("requested `%s' equals current state: %s", request, current)
	}
	return nil
}

func launchBlog(blog model.Blog, email string, s *model.Store) error {
	/* launch blog */
	if err := LaunchUserBlog(LaunchUserBlogParams{
		GhRepoFullName: blog.GhFullName,
		Subdomain:      blog.Subdomain,
	}); err != nil {
		return fmt.Errorf("error launching blog `%d': %w", blog.ID, err)
	}

	/* update status to live */
	if err := s.SetBlogStatusByID(context.TODO(), model.SetBlogStatusByIDParams{
		ID:     blog.ID,
		Status: model.BlogStatusLive,
	}); err != nil {
		return fmt.Errorf("error setting status to %s: %w", model.BlogStatusLive, err)
	}
	return nil
}

func deleteBlog(blog model.Blog, s *model.Store) error {
	site := filepath.Join(
		config.Config.Progstack.WebsitesPath,
		blog.Subdomain,
	)
	if err := os.RemoveAll(site); err != nil {
		return fmt.Errorf("error deleting website `%s' from disk: %w", blog.Subdomain, err)
	}
	err := s.SetBlogStatusByID(context.TODO(), model.SetBlogStatusByIDParams{
		ID:     blog.ID,
		Status: model.BlogStatusOffline,
	})
	if err != nil {
		return fmt.Errorf("error setting status to `%s': %w", model.BlogStatusOffline, err)
	}
	return nil
}
