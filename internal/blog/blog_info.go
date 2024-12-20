package blog

import (
	"context"
	"fmt"
	"time"

	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
)

type BlogInfo struct {
	ID                   int32
	Domain               string
	Subdomain            string
	CustomDomain         string
	DomainUrl            string
	RepositoryUrl        string
	SubscriberMetricsUrl string
	MetricsUrl           string
	ConfigUrl            string
	Theme                string
	Type                 string
	Status               string
	TestBranch           string
	LiveBranch           string
	UpdatedAt            time.Time
	IsRepository         bool
	IsLive               bool
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

func ghurl(blog model.Blog) string {
	if blog.BlogType == model.BlogTypeRepository {
		return blog.GhUrl.String
	}
	return ""
}

func buildDomain(subdomain string) string {
	return fmt.Sprintf(
		"%s.%s",
		subdomain,
		config.Config.Progstack.ServiceName,
	)
}

func buildDomainUrl(subdomain string) string {
	return fmt.Sprintf(
		"%s://%s.%s",
		config.Config.Progstack.Protocol,
		subdomain,
		config.Config.Progstack.ServiceName,
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

func getBlogInfo(s *model.Store, blogID int32) (BlogInfo, error) {
	blog, err := s.GetBlogByID(context.TODO(), blogID)
	if err != nil {
		return BlogInfo{}, fmt.Errorf("query error: %w", err)
	}
	isLive, err := s.GetBlogIsLive(context.TODO(), blogID)
	if err != nil {
		return BlogInfo{}, fmt.Errorf("islive error: %w", err)
	}
	return BlogInfo{
		ID:                   blog.ID,
		Domain:               buildDomain(blog.Subdomain.String()),
		Subdomain:            blog.Subdomain.String(),
		DomainUrl:            buildDomainUrl(blog.Subdomain.String()),
		RepositoryUrl:        ghurl(blog),
		SubscriberMetricsUrl: buildSubscriberMetricsUrl(blog.ID),
		MetricsUrl:           buildMetricsUrl(blog.ID),
		ConfigUrl:            buildConfigUrl(blog.ID),
		TestBranch:           blog.TestBranch.String,
		LiveBranch:           blog.LiveBranch.String,
		Theme:                string(blog.Theme),
		Type:                 string(blog.BlogType),
		UpdatedAt:            blog.UpdatedAt,
		IsRepository:         blog.BlogType == model.BlogTypeRepository,
		IsLive:               isLive,
	}, nil
}
