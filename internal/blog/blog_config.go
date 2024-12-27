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
	"strconv"

	"github.com/xr0-org/progstack/internal/analytics"
	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

type BlogService struct {
	client   *httpclient.Client
	store    *model.Store
	mixpanel *analytics.MixpanelClientWrapper
}

func NewBlogService(
	client *httpclient.Client, store *model.Store,
	mixpanel *analytics.MixpanelClientWrapper,
) *BlogService {
	return &BlogService{
		client:   client,
		store:    store,
		mixpanel: mixpanel,
	}
}

/* Blog configuration page */
func (b *BlogService) Config(
	r request.Request,
) (response.Response, error) {
	logger := r.Logger()
	logger.Println("Blog Config handler...")

	r.MixpanelTrack("Config")

	sesh := r.Session()
	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, util.CreateCustomError("", http.StatusNotFound)
	}
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse int: %w", err)
	}
	blogInfo, err := getBlogInfo(b.store, int32(intBlogID))
	if err != nil {
		return nil, fmt.Errorf("get blog info: %w", err)
	}

	return response.NewTemplate(
		[]string{"blog_config.html"},
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
	), nil
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

/* Theme */

func (b *BlogService) ThemeSubmit(
	r request.Request,
) (response.Response, error) {
	logger := r.Logger()
	logger.Println("ThemeSubmit handler...")

	r.MixpanelTrack("ThemeSubmit")

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, util.CreateCustomError("", http.StatusNotFound)
	}
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse blogID: %w", err)
	}

	var req struct {
		Theme string `json:"theme"`
	}
	body, err := r.ReadBody()
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("decode body: %w", err)
	}

	theme, err := validateTheme(req.Theme)
	if err != nil {
		return nil, fmt.Errorf("validate theme: %w", err)
	}

	if err := b.store.SetBlogThemeByID(
		context.TODO(),
		model.SetBlogThemeByIDParams{
			ID:    int32(intBlogID),
			Theme: theme,
		},
	); err != nil {
		return nil, fmt.Errorf("set blog theme: %w", err)
	}

	return response.NewJson(struct {
		Message string `json:"message"`
	}{"Theme changed successsfully!"})
}

/* Git branch info */

func (b *BlogService) TestBranchSubmit(
	r request.Request,
) (response.Response, error) {
	logger := r.Logger()
	logger.Println("TestBranchSubmit handler...")

	r.MixpanelTrack("TestBranchSubmit")

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, util.CreateCustomError("", http.StatusNotFound)
	}
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse blogID: %w", err)
	}

	var req struct {
		Branch string `json:"branch"`
	}
	body, err := r.ReadBody()
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("decode body: %w", err)
	}

	/* XXX: validate input before writing to db */
	if err := b.store.SetTestBranchByID(
		context.TODO(),
		model.SetTestBranchByIDParams{
			ID: int32(intBlogID),
			TestBranch: sql.NullString{
				Valid:  true,
				String: req.Branch,
			},
		}); err != nil {
		return nil, fmt.Errorf("error updating branch info: %w", err)
	}

	return response.NewJson(struct {
		Message string `json:"message"`
	}{"Test branch submitted successfully!"})
}

func (b *BlogService) LiveBranchSubmit(
	r request.Request,
) (response.Response, error) {
	logger := r.Logger()
	logger.Println("LiveBranchSubmit handler...")

	r.MixpanelTrack("LiveBranchSubmit")

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, util.CreateCustomError("", http.StatusNotFound)
	}
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse blogID: %w", err)
	}

	/* submitted with form and no javascript */
	var req struct {
		Branch string `json:"branch"`
	}
	body, err := r.ReadBody()
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("decode body: %w", err)
	}

	/* XXX: validate input before wrinting to db */
	if err := b.store.SetLiveBranchByID(
		context.TODO(),
		model.SetLiveBranchByIDParams{
			ID: int32(intBlogID),
			LiveBranch: sql.NullString{
				Valid:  true,
				String: req.Branch,
			},
		},
	); err != nil {
		return nil, fmt.Errorf("set live branch: %w", err)
	}

	return response.NewJson(struct {
		Message string `json:"message"`
	}{"Live branch submitted successsfully!"})

}

func (b *BlogService) FolderSubmit(
	r request.Request,
) (response.Response, error) {
	logger := r.Logger()
	logger.Println("FolderSubmit handler...")

	r.MixpanelTrack("FolderSubmit")

	type fsresp struct {
		Message string `json:"message"`
	}
	if err := b.folderSubmit(r); err != nil {
		return response.NewJson(&fsresp{"An unexpected error occured"})
	}
	return response.NewJson(&fsresp{"Folder updated successfully!"})
}

func (b *BlogService) folderSubmit(r request.Request) error {
	logger := r.Logger()

	src, err := getUploadedFolderPath(r)
	if err != nil {
		return fmt.Errorf("get uploaded folder path: %w", err)
	}

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return util.CreateCustomError("", http.StatusNotFound)
	}
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return fmt.Errorf("parse blogID: %w", err)
	}

	blog, err := b.store.GetBlogByID(context.TODO(), int32(intBlogID))
	if err != nil {
		return fmt.Errorf("get blog `%d': %w", intBlogID, err)
	}

	assert.Assert(blog.FolderPath.Valid)
	if err := clearAndExtract(src, blog.FolderPath.String); err != nil {
		return fmt.Errorf("clear and extract: %w", err)
	}

	if _, err := setBlogToLive(&blog, b.store, logger); err != nil {
		return fmt.Errorf("set blog to live: %w", err)
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

func getUploadedFolderPath(r request.Request) (string, error) {
	logger := r.Logger()
	file, header, err := r.GetFormFile("folder")
	if err != nil {
		logger.Printf("error reading file: %v\n", err)
		return "", util.CreateCustomError(
			"Invalid file",
			http.StatusBadRequest,
		)
	}
	defer file.Close()

	if !isValidFileType(header.Filename) {
		logger.Printf(
			"invalid file extension for `%s'\n", header.Filename,
		)
		return "", util.CreateCustomError(
			"Must upload a .zip file",
			http.StatusBadRequest,
		)
	}

	tmpFile, err := os.CreateTemp("", "uploaded-*.zip")
	if err != nil {
		return "", fmt.Errorf("create tmp file: %w", err)
	}
	defer tmpFile.Close()
	if _, err = io.Copy(tmpFile, file); err != nil {
		return "", fmt.Errorf("copy upload to temp file: %w", err)
	}
	return tmpFile.Name(), nil
}

func (b *BlogService) SetStatusSubmit(
	r request.Request,
) (response.Response, error) {
	logger := r.Logger()
	logger.Println("SetStatusSubmit handler...")

	r.MixpanelTrack("SetStatusSubmit")

	type resp struct {
		Message string `json:"message"`
	}

	change, err := b.setStatusSubmit(r)
	if err != nil {
		var customErr *util.CustomError
		if errors.As(err, &customErr) {
			return response.NewJson(&resp{customErr.Error()})
		} else {
			return nil, fmt.Errorf("set status submit: %w", err)
		}
	}

	return response.NewJson(&resp{change.message()})
}

func (b *BlogService) setStatusSubmit(
	r request.Request,
) (*statusChangeResponse, error) {
	logger := r.Logger()

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, util.CreateCustomError("", http.StatusNotFound)
	}
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse int: %w", err)
	}

	var req struct {
		IsLive bool `json:"is_live"`
	}
	body, err := r.ReadBody()
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("decode body: %w", err)
	}

	change, err := handleStatusChange(
		int32(intBlogID),
		req.IsLive, r.Session().GetEmail(), b.store, logger,
	)
	if err != nil {
		return nil, fmt.Errorf("handle status change: %w", err)
	}
	return change, nil
}

type statusChangeResponse struct {
	_islive bool
}

func (resp *statusChangeResponse) message() string {
	if resp._islive {
		return "Site is live!"
	}
	return "Site has been taken offline successfully."
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
		return setBlogToOffline(&blog, s)
	}
}

func validateStatusChange(blogID int32, islive bool, s *model.Store) error {
	blogIsLive, err := s.GetBlogIsLive(context.TODO(), blogID)
	if err != nil {
		return fmt.Errorf("islive error: %w", err)
	}
	if islive == blogIsLive {
		return util.CreateCustomError(
			"cannot update to same state",
			http.StatusBadRequest,
		)
	}
	return nil
}

func setBlogToLive(
	b *model.Blog, s *model.Store, logger *log.Logger,
) (*statusChangeResponse, error) {
	if err := s.SetBlogToLive(context.TODO(), b.ID); err != nil {
		return nil, err
	}
	if _, err := GetFreshGeneration(b.ID, s); err != nil {
		return nil, fmt.Errorf("cannot generate: %w", err)
	}
	return &statusChangeResponse{true}, nil
}

func setBlogToOffline(
	b *model.Blog, s *model.Store,
) (*statusChangeResponse, error) {
	if err := s.SetBlogToOffline(context.TODO(), b.ID); err != nil {
		return nil, fmt.Errorf("cannot set offline: %w", err)
	}
	return &statusChangeResponse{false}, nil
}

func (b *BlogService) ConfigDomain(
	r request.Request,
) (response.Response, error) {
	logger := r.Logger()
	logger.Println("ConfigDomain handler...")

	r.MixpanelTrack("ConfigDomain")

	sesh := r.Session()
	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, fmt.Errorf("no blogID")
	}
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse blogID: %w", err)
	}
	blogInfo, err := getBlogInfo(b.store, int32(intBlogID))
	if err != nil {
		return nil, fmt.Errorf("blog info: %w", err)
	}
	return response.NewTemplate(
		[]string{"blog_custom_domain.html"},
		util.PageInfo{
			Data: struct {
				Title    string
				UserInfo *session.UserInfo
				ID       int32
				Blog     BlogInfo
			}{
				Title:    "Custom domain configuration",
				UserInfo: session.ConvertSessionToUserInfo(sesh),
				ID:       int32(intBlogID),
				Blog:     blogInfo,
			},
		},
		template.FuncMap{},
		logger,
	), nil
}

func (b *BlogService) SyncRepository(
	r request.Request,
) (response.Response, error) {
	logger := r.Logger()
	logger.Println("SyncRepository handler...")

	r.MixpanelTrack("SyncRepository")

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, util.CreateCustomError("", http.StatusNotFound)
	}
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse int: %w", err)
	}

	blog, err := b.store.GetBlogByID(context.TODO(), int32(intBlogID))
	if err != nil {
		return nil, fmt.Errorf("error getting blog `%d': %w", intBlogID, err)
	}
	if err := UpdateRepositoryOnDisk(
		b.client, b.store, &blog, logger,
	); err != nil {
		return nil, fmt.Errorf("update error: %w", err)
	}

	return response.NewRedirect(
		fmt.Sprintf(
			"%s://%s/user/blogs/%d/config",
			config.Config.Progstack.Protocol,
			config.Config.Progstack.ServiceName,
			blog.ID,
		),
		http.StatusTemporaryRedirect,
	), nil
}
