package blog

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/blog/emaildata"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

type SiteData struct {
	CumulativeCounts template.JS
}

func (b *BlogService) SiteMetrics(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("SiteMetrics handler...")

	r.MixpanelTrack("SiteMetrics")

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
		return nil, fmt.Errorf("get blog: %w", err)
	}
	data, err := b.getSiteMetrics(blog.ID)
	if err != nil {
		return nil, fmt.Errorf("get site metrics: %w", err)
	}
	return response.NewTemplate(
		[]string{"site_metrics.html", "posts.html"},
		util.PageInfo{
			Data: struct {
				Title    string
				SiteName string
				IsLive   bool
				UserInfo *session.UserInfo
				SiteData
				PostData []postdata
			}{
				Title:    "Dashboard",
				SiteName: getsitename(&blog),
				IsLive:   blog.IsLive,
				UserInfo: session.ConvertSessionToUserInfo(sesh),
				PostData: data,
			},
		},
	), nil
}

type postdata struct {
	Title   string
	Url     template.URL
	Date    *time.Time
	NewSubs int
	Views   int
	Email   emaildata.EmailData
}

func (b *BlogService) getSiteMetrics(blogid int32) ([]postdata, error) {
	blog, err := b.store.GetBlogByID(context.TODO(), blogid)
	if err != nil {
		return nil, fmt.Errorf("cannot get blog: %w", err)
	}
	posts, err := b.store.ListPostsByBlog(context.TODO(), blogid)
	if err != nil {
		return nil, fmt.Errorf("cannot list posts: %w", err)
	}
	data := make([]postdata, len(posts))
	for i, p := range posts {
		u, err := url.JoinPath(
			fmt.Sprintf(
				"%s://%s.%s",
				config.Config.Progstack.Protocol,
				blog.Subdomain,
				config.Config.Progstack.RootDomain,
			),
			p.Url,
		)
		if err != nil {
			return nil, fmt.Errorf("url error: %w", err)
		}
		visits, err := b.store.CountVisits(
			context.TODO(),
			model.CountVisitsParams{
				Blog: blogid,
				Url:  p.Url,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("cannot count visits: %w", err)
		}
		emailopens, err := b.store.CountEmailClicks(
			context.TODO(),
			model.CountEmailClicksParams{
				Blog: blogid,
				Url:  p.Url,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("cannot count clicks: %w", err)
		}
		data[i] = postdata{
			Title: p.Title,
			Url:   template.URL(u),
			Date:  unwrapsqltime(p.PublishedAt),
			Views: int(visits),
			Email: getemaildata(&posts[i], int(emailopens)),
		}
	}
	return data, nil
}

func getemaildata(post *model.Post, clicks int) emaildata.EmailData {
	if post.EmailSent {
		return emaildata.NewSent(clicks)
	}
	return emaildata.NewUnsent(
		template.URL(
			fmt.Sprintf(
				"%s://%s/user/blogs/%d/email?token=%s",
				config.Config.Progstack.Protocol,
				config.Config.Progstack.RootDomain,
				post.Blog,
				post.EmailToken,
			),
		),
	)
}

func unwrapsqltime(time sql.NullTime) *time.Time {
	if time.Valid {
		return &time.Time
	}
	return nil
}
