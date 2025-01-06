package blog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/xr0-org/progstack-ssg/pkg/ssg"
	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/authn"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/dns"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
)

type CreateBlogResponse struct {
	Url     string `json:"url"`
	Message string `json:"message"`
}

func (b *BlogService) CreateRepositoryBlog(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("CreateRepositoryBlog handler...")

	r.MixpanelTrack("CreateRepositoryBlog")

	var req struct {
		Subdomain    string `json:"subdomain"`
		RepositoryID string `json:"repository_id"`
		Theme        string `json:"theme"`
		LiveBranch   string `json:"live_branch"`
		Flow         string `json:"flow"`
	}
	body, err := r.ReadBody()
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, createCustomError(
			"error decoding request body",
			http.StatusBadRequest,
		)
	}

	intRepoID, err := strconv.ParseInt(req.RepositoryID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf(
			"convert repositoryID `%s' to int64: %w",
			req.RepositoryID, err,
		)
	}

	theme, err := validateTheme(req.Theme)
	if err != nil {
		return nil, fmt.Errorf("validate theme: %w", err)
	}

	sub, err := dns.ParseSubdomain(req.Subdomain)
	if err != nil {
		return nil, fmt.Errorf("parse subdomain: %w", err)
	}

	if err := b.store.ExecTx(
		func(s *model.Store) error {
			return createBlogTx(
				intRepoID, &theme, sub, req.LiveBranch,
				b.client, sesh, s,
			)
		},
	); err != nil {
		return nil, fmt.Errorf("create blog tx: %w", err)
	}
	return response.NewJson(
		CreateBlogResponse{
			Url:     buildUrl(sub.String()),
			Message: "Successfully created repository-based blog!",
		},
	)
}

func createBlogTx(
	ghRepoID int64, theme *model.BlogTheme, sub *dns.Subdomain,
	livebranch string,
	c *httpclient.Client, sesh *session.Session, s *model.Store,
) error {
	userid, err := sesh.GetUserID()
	if err != nil {
		return fmt.Errorf("get user id: %w", err)
	}

	blog, err := s.CreateBlog(
		context.TODO(),
		model.CreateBlogParams{
			UserID:         userid,
			GhRepositoryID: ghRepoID,
			Theme:          *theme,
			Subdomain:      sub,
			LiveBranch:     livebranch,
			EmailMode:      model.EmailModeHtml,
			FromAddress: fmt.Sprintf(
				"%s@%s",
				sub, config.Config.Progstack.EmailDomain,
			),
		},
	)
	if err != nil {
		return fmt.Errorf("create blog: %w", err)
	}

	if err := UpdateRepositoryOnDisk(c, &blog, sesh, s); err != nil {
		return fmt.Errorf("update repo on disk: %w", err)
	}

	/* add owner as subscriber */
	if _, err = s.CreateSubscriber(
		context.TODO(),
		model.CreateSubscriberParams{
			BlogID: blog.ID,
			Email:  sesh.GetEmail(),
		},
	); err != nil {
		return fmt.Errorf("subscribe owner: %w", err)
	}

	if _, err := setBlogToLive(&blog, sesh, s); err != nil {
		return fmt.Errorf("set blog to live: %w", err)
	}

	return nil
}

func buildRepositoryUrl(fullName string) string {
	return fmt.Sprintf(
		"https://github.com/%s/",
		fullName,
	)
}

func validateTheme(theme string) (model.BlogTheme, error) {
	switch theme {
	case "lit":
		return model.BlogThemeLit, nil
	case "latex":
		return model.BlogThemeLatex, nil
	default:
		return "", fmt.Errorf("`%s' is not a supported theme", theme)
	}
}

func UpdateRepositoryOnDisk(
	c *httpclient.Client, blog *model.Blog,
	sesh *session.Session, s *model.Store,
) error {
	repo, err := s.GetRepositoryByGhRepositoryID(
		context.TODO(), blog.GhRepositoryID,
	)
	if err != nil {
		return fmt.Errorf("get repo: %w", err)
	}
	accessToken, err := authn.GetInstallationAccessToken(
		c,
		config.Config.Github.AppID,
		repo.InstallationID,
		config.Config.Github.PrivateKeyPath,
	)
	if err != nil {
		return fmt.Errorf("access token: %w", err)
	}
	if err := cloneRepo(
		repo.PathOnDisk,
		repo.Url,
		blog.LiveBranch,
		accessToken,
	); err != nil {
		return fmt.Errorf("clone: %w", err)
	}
	h, err := ssg.GetSiteHash(repo.PathOnDisk)
	if err != nil {
		return fmt.Errorf("get site hash: %w", err)
	}
	if err := s.UpdateBlogLiveHash(
		context.TODO(),
		model.UpdateBlogLiveHashParams{
			ID:       blog.ID,
			LiveHash: h,
		},
	); err != nil {
		return fmt.Errorf("update live hash: %w", err)
	}
	return nil
}
