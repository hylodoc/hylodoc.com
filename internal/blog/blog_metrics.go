package blog

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"github.com/hylodoc/hylodoc.com/internal/app/handler/request"
	"github.com/hylodoc/hylodoc.com/internal/app/handler/response"
	"github.com/hylodoc/hylodoc.com/internal/authz"
	"github.com/hylodoc/hylodoc.com/internal/blog/emaildata"
	"github.com/hylodoc/hylodoc.com/internal/config"
	"github.com/hylodoc/hylodoc.com/internal/model"
	"github.com/hylodoc/hylodoc.com/internal/session"
	"github.com/hylodoc/hylodoc.com/internal/util"
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

	blog, err := b.store.GetBlogByID(context.TODO(), blogID)
	if err != nil {
		return nil, fmt.Errorf("get blog: %w", err)
	}
	data, err := b.getSiteMetrics(blog.ID)
	if err != nil {
		return nil, fmt.Errorf("get site metrics: %w", err)
	}
	userID, err := sesh.GetUserID()
	if err != nil {
		return nil, fmt.Errorf("get user id: %w", err)
	}
	canView, err := authz.HasAnalyticsCustomDomainsImagesEmails(
		b.store, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("can view analytics: %w", err)
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
				PostData                      []postdata
				CanViewAnalyticsAndSendEmails bool
				UpgradeURL                    string
			}{
				Title:                         "Dashboard",
				SiteName:                      getsitename(&blog),
				IsLive:                        blog.IsLive,
				UserInfo:                      session.ConvertSessionToUserInfo(sesh),
				PostData:                      data,
				CanViewAnalyticsAndSendEmails: canView,
				UpgradeURL: fmt.Sprintf(
					"%s://%s/pricing",
					config.Config.Hylodoc.Protocol,
					config.Config.Hylodoc.RootDomain,
				),
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

func (b *BlogService) getSiteMetrics(blogid string) ([]postdata, error) {
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
				config.Config.Hylodoc.Protocol,
				blog.Subdomain,
				config.Config.Hylodoc.RootDomain,
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
				"%s://%s/user/blogs/%s/email?token=%s",
				config.Config.Hylodoc.Protocol,
				config.Config.Hylodoc.RootDomain,
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
