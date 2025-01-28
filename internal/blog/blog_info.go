package blog

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	"github.com/hylodoc/hylodoc.com/internal/config"
	"github.com/hylodoc/hylodoc.com/internal/model"
)

type BlogInfo struct {
	ID                       string
	Name                     string
	Subdomain                string
	Domain                   *string
	Url                      string
	ConfigureCustomDomainUrl string
	SetDomainUrl             string
	RepositoryUrl            string
	SubscriberMetricsUrl     string
	MetricsUrl               string
	ConfigUrl                string
	DeleteUrl                string
	Theme                    string
	Status                   string
	LiveBranch               string
	UpdatedAt                time.Time
	IsLive                   bool
	IsEmailModeHtml          bool
	Hash                     string
	HashUrl                  string
	SyncUrl                  string
	DeleteMessage            string
}

func GetBlogsInfo(s *model.Store, userID string) ([]BlogInfo, error) {
	ids, err := s.ListBlogIDsByUserID(context.TODO(), userID)
	if err != nil {
		/* should not be possible to have an installation with no repositories */
		return []BlogInfo{}, fmt.Errorf("list error: %w", err)
	}
	var info []BlogInfo
	for _, id := range ids {
		bi, err := getBlogInfo(s, id)
		if err != nil {
			return nil, fmt.Errorf("blog error: %w", err)
		}
		info = append(info, bi)
	}
	return info, nil
}

func getghurl(blog *model.Blog, s *model.Store) (*url.URL, error) {
	repo, err := s.GetRepositoryByGhRepositoryID(
		context.TODO(), blog.GhRepositoryID,
	)
	if err != nil {
		return nil, fmt.Errorf("get repo: %w", err)
	}
	u, err := url.Parse(repo.Url)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return u, nil
}

func getname(blog *model.Blog) string {
	if blog.Name.Valid {
		return blog.Name.String
	}
	if blog.Domain.Valid {
		return blog.Domain.String
	}
	return blog.Subdomain.String()
}

func buildUrl(subdomain string) string {
	return fmt.Sprintf(
		"%s://%s.%s",
		config.Config.Hylodoc.Protocol,
		subdomain,
		config.Config.Hylodoc.RootDomain,
	)
}

func buildConfigureCustomDomainUrl(blogID string) string {
	return fmt.Sprintf(
		"/user/blogs/%s/config-domain",
		blogID,
	)
}

func buildSetDomainUrl(blogID string) string {
	return fmt.Sprintf(
		"/user/blogs/%s/set-domain",
		blogID,
	)
}

func buildSubscriberMetricsUrl(blogID string) string {
	return fmt.Sprintf(
		"/user/blogs/%s/subscriber/metrics",
		blogID,
	)
}

func buildMetricsUrl(blogID string) string {
	return fmt.Sprintf(
		"/user/blogs/%s/metrics",
		blogID,
	)
}

func buildConfigUrl(blogID string) string {
	return fmt.Sprintf(
		"/user/blogs/%s/config",
		blogID,
	)
}

func buildDeleteUrl(blogID string) string {
	return fmt.Sprintf(
		"/user/blogs/%s/delete",
		blogID,
	)
}

func buildSyncUrl(blogID string) string {
	return fmt.Sprintf(
		"/user/blogs/%s/sync",
		blogID,
	)
}

func getBlogInfo(s *model.Store, blogID string) (BlogInfo, error) {
	blog, err := s.GetBlogByID(context.TODO(), blogID)
	if err != nil {
		return BlogInfo{}, fmt.Errorf("query error: %w", err)
	}
	isLive, err := s.GetBlogIsLive(context.TODO(), blogID)
	if err != nil {
		return BlogInfo{}, fmt.Errorf("islive error: %w", err)
	}
	isEmailModeHtml, err := blogEmailModeIsHtml(blog.EmailMode)
	if err != nil {
		return BlogInfo{}, fmt.Errorf("blog email mode is html: %w", err)
	}
	ghurl, err := getghurl(&blog, s)
	if err != nil {
		return BlogInfo{}, fmt.Errorf("ghurl: %w", err)
	}
	return BlogInfo{
		ID:                       blog.ID,
		Name:                     getname(&blog),
		Domain:                   unwrapsqlnullstr(blog.Domain),
		Subdomain:                blog.Subdomain.String(),
		Url:                      buildUrl(blog.Subdomain.String()),
		ConfigureCustomDomainUrl: buildConfigureCustomDomainUrl(blog.ID),
		SetDomainUrl:             buildSetDomainUrl(blog.ID),
		RepositoryUrl:            ghurl.String(),
		SubscriberMetricsUrl:     buildSubscriberMetricsUrl(blog.ID),
		MetricsUrl:               buildMetricsUrl(blog.ID),
		ConfigUrl:                buildConfigUrl(blog.ID),
		DeleteUrl:                buildDeleteUrl(blog.ID),
		LiveBranch:               blog.LiveBranch,
		Theme:                    string(blog.Theme),
		UpdatedAt:                blog.UpdatedAt,
		IsLive:                   isLive,
		IsEmailModeHtml:          isEmailModeHtml,
		SyncUrl:                  buildSyncUrl(blog.ID),
		Hash:                     blog.LiveHash.String,
		HashUrl: ghurl.JoinPath(
			"commit", blog.LiveHash.String,
		).String(),
		DeleteMessage: blogDeleteMessage(&blog),
	}, nil
}

func blogEmailModeIsHtml(emailMode model.EmailMode) (bool, error) {
	switch emailMode {
	case model.EmailModeHtml:
		return true, nil
	case model.EmailModePlaintext:
		return false, nil
	default:
		return false, fmt.Errorf(
			"invalid email mode: %s", string(emailMode),
		)
	}
}

func blogDeleteMessage(b *model.Blog) string {
	return fmt.Sprintf("I am certain, delete %s.", getname(b))
}

func unwrapsqlnullstr(s sql.NullString) *string {
	if s.Valid {
		return &s.String
	}
	return nil
}
