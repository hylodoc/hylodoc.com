package blog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/hylodoc/hylodoc.com/internal/app/handler/request"
	"github.com/hylodoc/hylodoc.com/internal/app/handler/response"
	"github.com/hylodoc/hylodoc.com/internal/authz"
	"github.com/hylodoc/hylodoc.com/internal/config"
	"github.com/hylodoc/hylodoc.com/internal/httpclient"
	"github.com/hylodoc/hylodoc.com/internal/model"
	"github.com/hylodoc/hylodoc.com/internal/session"
	"github.com/hylodoc/hylodoc.com/internal/util"
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

	blogInfo, err := getBlogInfo(b.store, blogID)
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
				ID              string
				Blog            BlogInfo
				Themes          []string
				CurrentTheme    string
				CanCustomDomain bool
				UpgradeURL      string
			}{
				Title:           "Blog Setup",
				UserInfo:        session.ConvertSessionToUserInfo(sesh),
				ID:              blogID,
				Blog:            blogInfo,
				Themes:          BuildThemes(config.Config.Knu.Themes),
				CurrentTheme:    string(blogInfo.Theme),
				CanCustomDomain: canConfigure,
				UpgradeURL: fmt.Sprintf(
					"%s://%s/pricing",
					config.Config.Hylodoc.Protocol,
					config.Config.Hylodoc.RootDomain,
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

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, createCustomError("", http.StatusNotFound)
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
	blogID string, theme model.BlogTheme, tx *model.Store,
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
	if err := regenerateBlog(blogID, tx); err != nil {
		return fmt.Errorf("regenerate blog: %w", err)
	}
	return nil
}

func regenerateBlog(blogID string, s *model.Store) error {
	if err := s.MarkBlogGenerationsStale(
		context.TODO(), blogID,
	); err != nil {
		return fmt.Errorf("mark blog generations stale: %w", err)
	}
	if _, err := GetFreshGeneration(blogID, s); err != nil {
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

	if err := b.store.ExecTx(
		func(tx *model.Store) error {
			return updateBlogLiveBranchTx(
				blogID, req.Branch, tx, b.client, sesh,
			)
		},
	); err != nil {
		return nil, fmt.Errorf("update blog live branch tx: %w", err)
	}

	return response.NewJson(struct {
		Message string `json:"message"`
	}{"Live branch submitted successsfully!"})

}

func updateBlogLiveBranchTx(
	blogID string, branch string,
	tx *model.Store, c *httpclient.Client, sesh *session.Session,
) error {
	/* TODO: validate input before wrinting to db */
	if err := tx.SetLiveBranchByID(
		context.TODO(),
		model.SetLiveBranchByIDParams{
			ID:         blogID,
			LiveBranch: branch,
		},
	); err != nil {
		return fmt.Errorf("set live branch: %w", err)
	}
	b, err := tx.GetBlogByID(context.TODO(), blogID)
	if err != nil {
		return fmt.Errorf("get blog: %w", err)
	}
	if err := UpdateRepositoryOnDisk(c, &b, sesh, tx); err != nil {
		return fmt.Errorf("update repo on disk: %w", err)
	}
	if err := regenerateBlog(blogID, tx); err != nil {
		return fmt.Errorf("regenerate blog: %w", err)
	}
	return nil
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
		blogID, req.IsLive, r.Session().GetEmail(), b.store, sesh,
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
	blogID string, islive bool, email string, s *model.Store, sesh *session.Session,
) (*statusChangeResponse, error) {
	blog, err := s.GetBlogByID(context.TODO(), blogID)
	if err != nil {
		return nil, fmt.Errorf("error getting blog `%s': %w", blogID, err)
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

func validateStatusChange(blogID string, islive bool, s *model.Store) error {
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

	blogInfo, err := getBlogInfo(b.store, blogID)
	if err != nil {
		return nil, fmt.Errorf("blog info: %w", err)
	}
	return response.NewTemplate(
		[]string{"blog_custom_domain.html"},
		util.PageInfo{
			Data: struct {
				Title    string
				UserInfo *session.UserInfo
				ID       string
				Blog     BlogInfo
			}{
				Title:    "Custom domain configuration",
				UserInfo: session.ConvertSessionToUserInfo(sesh),
				ID:       blogID,
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

	blog, err := b.store.GetBlogByID(context.TODO(), blogID)
	if err != nil {
		return nil, fmt.Errorf("error getting blog `%s': %w", blogID, err)
	}
	if err := UpdateRepositoryOnDisk(
		b.client, &blog, sesh, b.store,
	); err != nil {
		return nil, fmt.Errorf("update error: %w", err)
	}

	return response.NewRedirect(
		fmt.Sprintf(
			"%s://%s/user/blogs/%s/config",
			config.Config.Hylodoc.Protocol,
			config.Config.Hylodoc.RootDomain,
			blog.ID,
		),
		http.StatusTemporaryRedirect,
	), nil
}

func (b *BlogService) Delete(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("Blog Delete handler...")

	r.MixpanelTrack("BlogDelete")

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, createCustomError("", http.StatusNotFound)
	}

	blog, err := b.store.GetBlogByID(context.TODO(), blogID)
	if err != nil {
		return nil, fmt.Errorf("get blog: %w", err)
	}

	message, err := r.GetPostFormValue("message")
	if err != nil {
		sesh.Printf("cannot get post form value: %v\n", err)
		return nil, createCustomError(
			"Error parsing form", http.StatusBadRequest,
		)
	}
	if message != blogDeleteMessage(&blog) {
		sesh.Printf("wrong delete message %q\n", message)
		return response.NewRedirect(
			fmt.Sprintf(
				"%s://%s/user/blogs/%s/config",
				config.Config.Hylodoc.Protocol,
				config.Config.Hylodoc.RootDomain,
				blogID,
			),
			http.StatusTemporaryRedirect,
		), nil
	}
	if err := b.store.MarkBlogGenerationsStale(
		context.TODO(), blog.ID,
	); err != nil {
		return nil, fmt.Errorf("mark blog generations stale: %w", err)
	}
	if err := b.store.DeleteBlogByID(context.TODO(), blog.ID); err != nil {
		return nil, fmt.Errorf("delete blog: %w", err)
	}

	/* redirect to /home */
	return response.NewRedirect(
		fmt.Sprintf(
			"%s://%s/user/",
			config.Config.Hylodoc.Protocol,
			config.Config.Hylodoc.RootDomain,
		),
		http.StatusTemporaryRedirect,
	), nil
}
