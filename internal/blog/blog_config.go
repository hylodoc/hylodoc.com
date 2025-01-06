package blog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/authz"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

type BlogService struct {
	client *httpclient.Client
	store  *model.Store
}

func NewBlogService(
	client *httpclient.Client, store *model.Store,
) *BlogService {
	return &BlogService{client, store}
}

/* Blog configuration page */
func (b *BlogService) Config(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("Blog Config handler...")

	r.MixpanelTrack("Config")

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, createCustomError("", http.StatusNotFound)
	}
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse int: %w", err)
	}
	blogInfo, err := getBlogInfo(b.store, int32(intBlogID))
	if err != nil {
		return nil, fmt.Errorf("get blog info: %w", err)
	}
	userID, err := sesh.GetUserID()
	if err != nil {
		return nil, fmt.Errorf("get user id: %w", err)
	}
	canConfigure, err := authz.HasAnalyticsCustomDomainsImagesEmails(
		b.store, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("can configure custom domain: %w", err)
	}

	return response.NewTemplate(
		[]string{"blog_config.html"},
		util.PageInfo{
			Data: struct {
				Title           string
				UserInfo        *session.UserInfo
				ID              int32
				Blog            BlogInfo
				Themes          []string
				CurrentTheme    string
				CanCustomDomain bool
				UpgradeURL      string
			}{
				Title:           "Blog Setup",
				UserInfo:        session.ConvertSessionToUserInfo(sesh),
				ID:              int32(intBlogID),
				Blog:            blogInfo,
				Themes:          BuildThemes(config.Config.ProgstackSsg.Themes),
				CurrentTheme:    string(blogInfo.Theme),
				CanCustomDomain: canConfigure,
				UpgradeURL: fmt.Sprintf(
					"%s://%s/pricing",
					config.Config.Progstack.Protocol,
					config.Config.Progstack.RootDomain,
				),
			},
		},
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
	sesh := r.Session()
	sesh.Println("ThemeSubmit handler...")

	r.MixpanelTrack("ThemeSubmit")

	rawBlogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, createCustomError("", http.StatusNotFound)
	}
	intBlogID, err := strconv.ParseInt(rawBlogID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse blogID: %w", err)
	}
	blogID := int32(intBlogID)

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

	theme, err := getTheme(req.Theme)
	if err != nil {
		return nil, fmt.Errorf("get theme: %w", err)
	}
	if err := b.store.ExecTx(
		func(tx *model.Store) error {
			return updateBlogThemeTx(blogID, theme, tx)
		},
	); err != nil {
		return nil, fmt.Errorf("update blog theme tx: %w", err)
	}

	return response.NewJson(struct {
		Message string `json:"message"`
	}{"Theme changed successsfully!"})
}

func updateBlogThemeTx(
	blogID int32, theme model.BlogTheme, tx *model.Store,
) error {
	if err := tx.SetBlogThemeByID(
		context.TODO(),
		model.SetBlogThemeByIDParams{
			ID:    blogID,
			Theme: theme,
		},
	); err != nil {
		return fmt.Errorf("set blog theme: %w", err)
	}
	if err := tx.MarkBlogGenerationsStale(
		context.TODO(), blogID,
	); err != nil {
		return fmt.Errorf("mark blog generations stale: %w", err)
	}
	if _, err := GetFreshGeneration(blogID, tx); err != nil {
		return fmt.Errorf("get fresh generation: %w", err)
	}
	return nil
}

/* Git branch info */

func (b *BlogService) LiveBranchSubmit(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("LiveBranchSubmit handler...")

	r.MixpanelTrack("LiveBranchSubmit")

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, createCustomError("", http.StatusNotFound)
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
			ID:         int32(intBlogID),
			LiveBranch: req.Branch,
		},
	); err != nil {
		return nil, fmt.Errorf("set live branch: %w", err)
	}

	return response.NewJson(struct {
		Message string `json:"message"`
	}{"Live branch submitted successsfully!"})

}

func (b *BlogService) SetStatusSubmit(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("SetStatusSubmit handler...")

	r.MixpanelTrack("SetStatusSubmit")

	type resp struct {
		Message string `json:"message"`
	}

	change, err := b.setStatusSubmit(r)
	if err != nil {
		var customErr *customError
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
	sesh := r.Session()

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, createCustomError("", http.StatusNotFound)
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
		req.IsLive, r.Session().GetEmail(), b.store, sesh,
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
	blogID int32, islive bool, email string, s *model.Store, sesh *session.Session,
) (*statusChangeResponse, error) {
	blog, err := s.GetBlogByID(context.TODO(), blogID)
	if err != nil {
		return nil, fmt.Errorf("error getting blog `%d': %w", blogID, err)
	}
	if err := validateStatusChange(blogID, islive, s); err != nil {
		return nil, fmt.Errorf("invalid status change: %w", err)
	}
	if islive {
		return setBlogToLive(&blog, sesh, s)
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
		return createCustomError(
			"cannot update to same state",
			http.StatusBadRequest,
		)
	}
	return nil
}

func setBlogToLive(
	b *model.Blog, sesh *session.Session, s *model.Store,
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
	sesh := r.Session()
	sesh.Println("ConfigDomain handler...")

	r.MixpanelTrack("ConfigDomain")

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
	), nil
}

func (b *BlogService) SyncRepository(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("SyncRepository handler...")

	r.MixpanelTrack("SyncRepository")

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, createCustomError("", http.StatusNotFound)
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
		b.client, &blog, sesh, b.store,
	); err != nil {
		return nil, fmt.Errorf("update error: %w", err)
	}

	return response.NewRedirect(
		fmt.Sprintf(
			"%s://%s/user/blogs/%d/config",
			config.Config.Progstack.Protocol,
			config.Config.Progstack.RootDomain,
			blog.ID,
		),
		http.StatusTemporaryRedirect,
	), nil
}
