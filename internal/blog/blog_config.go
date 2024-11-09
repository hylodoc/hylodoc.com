package blog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack-ssg/pkg/ssg"
	"github.com/xr0-org/progstack/internal/analytics"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

type BlogService struct {
	client       *httpclient.Client
	store        *model.Store
	resendClient *resend.Client
	mixpanel     *analytics.MixpanelClientWrapper
}

func NewBlogService(
	client *httpclient.Client, store *model.Store,
	resendClient *resend.Client, mixpanel *analytics.MixpanelClientWrapper,
) *BlogService {
	return &BlogService{
		client:       client,
		store:        store,
		resendClient: resendClient,
		mixpanel:     mixpanel,
	}
}

/* Blog configuration page */
func (b *BlogService) Config() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("Blog Config handler...")

		b.mixpanel.Track("Config", r)

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}
		blogID := mux.Vars(r)["blogID"]
		intBlogID, err := strconv.ParseInt(blogID, 10, 32)
		if err != nil {
			logging.Logger(r).Printf("error converting string path var to blogID: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		blogInfo, err := getBlogInfo(b.store, int32(intBlogID))
		if err != nil {
			logging.Logger(r).Printf("error getting blog for user `%d': %v\n", sesh.GetUserID(), err)
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
			logger,
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

func launchUserBlog(s *model.Store, b *model.Blog) error {
	if _, err := os.Stat(b.RepositoryPath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf(
				"repository at `%s' does not exist on disk: %w",
				b.RepositoryPath, err,
			)
		}
		return err
	}
	bindings, err := ssg.GenerateSiteWithBindings(
		b.RepositoryPath,
		filepath.Join(
			config.Config.Progstack.WebsitesPath,
			b.Subdomain,
		),
		config.Config.ProgstackSsg.Themes[string(b.Theme)].Path,
		"algol_nu",
		"",
		"Subscribe via <a href=\"/subscribe\">email</a>.",
		map[string]ssg.CustomPage{
			"/subscribe": ssg.NewSubscriberPage(
				fmt.Sprintf(
					"http://%s/blogs/%d/subscribe",
					baseurl, b.ID,
				),
			),
			"/subscribed": ssg.NewMessagePage(
				"Subscribed",
				"You have been subscribed. Please check your email.",
			),
		},
	)
	if err != nil {
		return fmt.Errorf("error generating site: %w", err)
	}
	gen, err := s.InsertGeneration(context.TODO(), b.ID)
	if err != nil {
		return fmt.Errorf("error inserting generation: %w", err)
	}
	for url, file := range bindings {
		if err := s.InsertBinding(
			context.TODO(),
			model.InsertBindingParams{
				Gen:  gen,
				Url:  url,
				File: file.Path(),
			},
		); err != nil {
			return fmt.Errorf("error inserting binding: %w", err)
		}
		if file.IsPost() {
			if err := upsertPostDetails(
				s, url, b.ID, file,
			); err != nil {
				return fmt.Errorf(
					"error ensuring post exists: %w", err,
				)
			}
		}
	}
	return nil
}

func upsertPostDetails(
	s *model.Store, url string, blogid int32, file ssg.File,
) error {
	published := publishedat(file)
	_, err := s.GetPostExists(
		context.TODO(),
		model.GetPostExistsParams{
			Url:  url,
			Blog: blogid,
		},
	)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("error checking if post exists: %w", err)
		}
		return s.InsertRPost(
			context.TODO(),
			model.InsertRPostParams{
				Url:         url,
				Blog:        blogid,
				PublishedAt: published,
				Title:       file.PostTitle(),
			},
		)
	}
	return s.UpdateRPost(
		context.TODO(),
		model.UpdateRPostParams{
			Url:         url,
			Blog:        blogid,
			PublishedAt: published,
			Title:       file.PostTitle(),
		},
	)
}

func publishedat(file ssg.File) sql.NullTime {
	t, ok := file.PostTime()
	return sql.NullTime{t, ok}
}

/* Theme */

func (b *BlogService) ThemeSubmit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("ThemeSubmit handler...")

		b.mixpanel.Track("ThemeSubmit", r)

		if err := b.themeSubmit(w, r); err != nil {
			logger.Printf("error updating theme: %v", err)
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
		logger := logging.Logger(r)
		logger.Println("TestBranchSubmit handler...")

		b.mixpanel.Track("TestBranchSubmit", r)

		if err := b.testBranchSubmit(w, r); err != nil {
			logger.Printf("error updating test branch: %v", err)
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
	logging.Logger(r).Printf("test branch: %s\n", req.Branch)

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
		logger := logging.Logger(r)
		logger.Println("LiveBranchSubmit handler...")

		b.mixpanel.Track("LiveBranchSubmit", r)

		if err := b.liveBranchSubmit(w, r); err != nil {
			logger.Printf("Error updating live branch: %v", err)
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
	logger := logging.Logger(r)

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
	logger.Printf("live branch: %s\n", req.Branch)

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

func (b *BlogService) FolderSubmit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("FolderSubmit handler...")

		b.mixpanel.Track("FolderSubmit", r)

		if err := b.folderSubmit(w, r); err != nil {
			logger.Printf("error update folder: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			response := map[string]string{"message": "An unexpected error occured"}
			json.NewEncoder(w).Encode(response)
			return
		}
		w.WriteHeader(http.StatusOK)
		response := map[string]string{"message": "Folder updated successfully!"}
		json.NewEncoder(w).Encode(response)
	}
}

func (b *BlogService) folderSubmit(w http.ResponseWriter, r *http.Request) error {
	logger := logging.Logger(r)

	_, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		logger.Println("user not found")
		return util.CreateCustomError("", http.StatusNotFound)
	}

	src, err := parseFolderUpdateRequest(r)
	if err != nil {
		return err
	}

	blogID := mux.Vars(r)["blogID"]
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return err
	}

	blog, err := b.store.GetBlogByID(context.TODO(), int32(intBlogID))
	if err != nil {
		return fmt.Errorf("error getting blog `%d': %w", intBlogID, err)
	}

	logger.Printf("src: %s\n", src)
	logger.Printf("dst: %s\n", blog.RepositoryPath)

	/* extract to appropriate path for folders */
	if err := clearAndExtract(src, blog.RepositoryPath); err != nil {
		return err
	}

	/* take blog live  */
	_, err = setBlogToLive(&blog, b.store, logger)
	if err != nil {
		return fmt.Errorf("error setting blog to live: %w", err)
	}
	return nil
}

func clearAndExtract(src, dst string) error {
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("failed to delete directory %s: %w", dst, err)
	}
	if err := os.MkdirAll(dst, os.ModePerm); err != nil {
		return fmt.Errorf("failed to recreate directory %s: %w", dst, err)
	}
	if err := extractZip(src, dst); err != nil {
		return fmt.Errorf("error extracting .zip: %w", err)
	}
	return nil
}

func parseFolderUpdateRequest(r *http.Request) (string, error) {
	/* XXX: Add subscription based file size limits */
	err := r.ParseMultipartForm(maxFileSize) /* 10MB limit */
	if err != nil {
		logging.Logger(r).Printf("file too large: %v\n", err)
		return "", util.CreateCustomError(
			"File too large",
			http.StatusBadRequest,
		)
	}

	file, header, err := r.FormFile("folder")
	if err != nil {
		logging.Logger(r).Printf("error reading file: %v\n", err)
		return "", util.CreateCustomError(
			"Invalid file",
			http.StatusBadRequest,
		)
	}
	defer file.Close()

	if !isValidFileType(header.Filename) {
		logging.Logger(r).Printf("invalid file extension for `%s'\n", header.Filename)
		return "", util.CreateCustomError(
			"Must upload a .zip file",
			http.StatusBadRequest,
		)
	}

	/* create to tmp file */
	tmpFile, err := os.CreateTemp("", "uploaded-*.zip")
	if err != nil {
		return "", fmt.Errorf("error creating tmp file: %w", err)
	}
	defer tmpFile.Close()

	/* copy uploaded file to tmpFile */
	if _, err = io.Copy(tmpFile, file); err != nil {
		return "", fmt.Errorf("error copying upload to temp file: %w", err)
	}

	return tmpFile.Name(), nil
}

type SetStatusSubmitResponse struct {
	Message string `json:"message"`
}

func (b *BlogService) SetStatusSubmit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("SetStatusSubmit handler...")

		b.mixpanel.Track("SetStatusSubmit", r)

		w.Header().Set("Content-Type", "application/json")

		change, err := b.setStatusSubmit(r)
		if err != nil {
			var customErr *util.CustomError
			if errors.As(err, &customErr) {
				logger.Printf("Client Error: %v\n", customErr)
				w.WriteHeader(http.StatusBadRequest)
				if err := json.NewEncoder(w).Encode(util.ErrorResponse{
					Message: customErr.Error(),
				}); err != nil {
					logging.Logger(r).Printf("Failed to encode response: %v\n", err)
					http.Error(w, "", http.StatusInternalServerError)
					return
				}
			} else {
				logger.Printf("Internal Server Error: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				http.Error(w, "", http.StatusInternalServerError)
				return
			}
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
			logger.Printf("Failed to encode response: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}
}

type SetStatusRequest struct {
	IsLive bool `json:"is_live"`
}

func (b *BlogService) setStatusSubmit(
	r *http.Request,
) (*statusChangeResponse, error) {
	logger := logging.Logger(r)

	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		return nil, fmt.Errorf("no user found")
	}
	blogID := mux.Vars(r)["blogID"]
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("cannot parse blog id: %w", err)
	}

	var req SetStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("error decoding body: %w", err)
	}

	change, err := handleStatusChange(
		int32(intBlogID), req.IsLive, sesh.GetEmail(), b.store, logger,
	)
	if err != nil {
		return nil, fmt.Errorf("error handling status change: %w", err)
	}
	return change, nil
}

type statusChangeResponse struct {
	Domain string
	IsLive bool
}

func handleStatusChange(
	blogID int32, islive bool, email string, s *model.Store, logger *log.Logger,
) (*statusChangeResponse, error) {
	blog, err := s.GetBlogByID(context.TODO(), blogID)
	if err != nil {
		return nil, fmt.Errorf("error getting blog `%d': %w", blogID, err)
	}
	if err := validateStatusChange(blogID, islive, s); err != nil {
		return nil, fmt.Errorf("invalid status change: %w", err)
	}
	if islive {
		return setBlogToLive(&blog, s, logger)
	} else {
		return setBlogToOffline(blog, s)
	}
}

func validateStatusChange(blogID int32, islive bool, s *model.Store) error {
	blogIsLive, err := s.GetBlogIsLive(context.TODO(), blogID)
	if err != nil {
		return fmt.Errorf("islive error: %w", err)
	}
	if islive == blogIsLive {
		return util.CreateCustomError(
			fmt.Sprintf("cannot update to same state"),
			http.StatusBadRequest,
		)
	}
	return nil
}

func setBlogToLive(b *model.Blog, s *model.Store, logger *log.Logger) (*statusChangeResponse, error) {
	logger.Printf("repo disk path: %s\n", b.RepositoryPath)
	if err := launchUserBlog(s, b); err != nil {
		return nil, fmt.Errorf("error launching blog `%d': %w", b.ID, err)
	}
	return &statusChangeResponse{
		Domain: b.Subdomain,
		IsLive: true,
	}, nil
}

func setBlogToOffline(blog model.Blog, s *model.Store) (*statusChangeResponse, error) {
	site := filepath.Join(
		config.Config.Progstack.WebsitesPath,
		blog.Subdomain,
	)
	if err := os.RemoveAll(site); err != nil {
		return nil, fmt.Errorf(
			"error deleting website `%s' from disk: %w",
			blog.Subdomain, err,
		)
	}
	if err := s.DeactivateGenerations(context.TODO(), blog.ID); err != nil {
		return nil, fmt.Errorf("deactivate error: %w", err)
	}
	return &statusChangeResponse{
		Domain: blog.Subdomain,
		IsLive: false,
	}, nil
}
