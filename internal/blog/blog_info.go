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
	Type                 string
	Status               string
	TestBranch           string
	LiveBranch           string
	UpdatedAt            time.Time
	IsRepository         bool
	IsLive               bool
}

func GetBlogsInfo(s *model.Store, userID int32) ([]BlogInfo, error) {
	blogs, err := s.ListBlogsByUserID(context.TODO(), userID)
	if err != nil {
		/* should not be possible to have an installation with no repositories */
		return []BlogInfo{}, err
	}
	var info []BlogInfo
	for _, blog := range blogs {
		info = append(info, buildBlogInfo(blog))
	}
	return info, nil
}

func buildBlogInfo(blog model.Blog) BlogInfo {
	isRepository := false
	url := ""
	if blog.BlogType == model.BlogTypeRepository {
		isRepository = true
		url = blog.GhUrl.String
	}
	isLive := false
	if blog.Status == model.BlogStatusLive {
		isLive = true
	}
	return BlogInfo{
		ID:                   blog.ID,
		Domain:               buildDomain(blog.Subdomain),
		Subdomain:            blog.Subdomain,
		DomainUrl:            buildDomainUrl(blog.Subdomain),
		RepositoryUrl:        url,
		SubscriberMetricsUrl: buildSubscriberMetricsUrl(blog.ID),
		MetricsUrl:           buildMetricsUrl(blog.ID),
		ConfigUrl:            buildConfigUrl(blog.ID),
		TestBranch:           blog.TestBranch.String,
		LiveBranch:           blog.LiveBranch.String,
		Type:                 string(blog.BlogType),
		Status:               string(blog.Status),
		UpdatedAt:            blog.UpdatedAt,
		IsRepository:         isRepository,
		IsLive:               isLive,
	}
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
		return BlogInfo{}, err
	}
	return buildBlogInfo(blog), err
}
