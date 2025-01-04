package blog

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
)

type BlogInfo struct {
	ID                       int32
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
	Theme                    string
	Status                   string
	LiveBranch               string
	UpdatedAt                time.Time
	IsLive                   bool
	Hash                     string
	HashUrl                  string
	SyncUrl                  string
}

func GetBlogsInfo(s *model.Store, userID int32) ([]BlogInfo, error) {
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

func getname(blog model.Blog) string {
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
		config.Config.Progstack.Protocol,
		subdomain,
		config.Config.Progstack.RootDomain,
	)
}

func buildConfigureCustomDomainUrl(blogID int32) string {
	return fmt.Sprintf(
		"/user/blogs/%d/config-domain",
		blogID,
	)
}

func buildSetDomainUrl(blogID int32) string {
	return fmt.Sprintf(
		"/user/blogs/%d/set-domain",
		blogID,
	)
}

func buildSubscriberMetricsUrl(blogID int32) string {
	return fmt.Sprintf(
		"/user/blogs/%d/subscriber/metrics",
		blogID,
	)
}

func buildMetricsUrl(blogID int32) string {
	return fmt.Sprintf(
		"/user/blogs/%d/metrics",
		blogID,
	)
}

func buildConfigUrl(blogID int32) string {
	return fmt.Sprintf(
		"/user/blogs/%d/config",
		blogID,
	)
}

func buildSyncUrl(blogID int32) string {
	return fmt.Sprintf(
		"/user/blogs/%d/sync",
		blogID,
	)
}

func getBlogInfo(s *model.Store, blogID int32) (BlogInfo, error) {
	blog, err := s.GetBlogByID(context.TODO(), blogID)
	if err != nil {
		return BlogInfo{}, fmt.Errorf("query error: %w", err)
	}
	isLive, err := s.GetBlogIsLive(context.TODO(), blogID)
	if err != nil {
		return BlogInfo{}, fmt.Errorf("islive error: %w", err)
	}
	ghurl, err := getghurl(&blog, s)
	if err != nil {
		return BlogInfo{}, fmt.Errorf("ghurl: %w", err)
	}
	return BlogInfo{
		ID:                       blog.ID,
		Name:                     getname(blog),
		Domain:                   unwrapsqlnullstr(blog.Domain),
		Subdomain:                blog.Subdomain.String(),
		Url:                      buildUrl(blog.Subdomain.String()),
		ConfigureCustomDomainUrl: buildConfigureCustomDomainUrl(blog.ID),
		SetDomainUrl:             buildSetDomainUrl(blog.ID),
		RepositoryUrl:            ghurl.String(),
		SubscriberMetricsUrl:     buildSubscriberMetricsUrl(blog.ID),
		MetricsUrl:               buildMetricsUrl(blog.ID),
		ConfigUrl:                buildConfigUrl(blog.ID),
		LiveBranch:               blog.LiveBranch,
		Theme:                    string(blog.Theme),
		UpdatedAt:                blog.UpdatedAt,
		IsLive:                   isLive,
		SyncUrl:                  buildSyncUrl(blog.ID),
		Hash:                     blog.LiveHash.String,
		HashUrl: ghurl.JoinPath(
			"commit", blog.LiveHash.String,
		).String(),
	}, nil
}

func unwrapsqlnullstr(s sql.NullString) *string {
	if s.Valid {
		return &s.String
	}
	return nil
}
