package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/blog/emaildata"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

type SiteData struct {
	CumulativeCounts template.JS
}

func (b *BlogService) SiteMetrics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("SiteMetrics handler...")

		b.mixpanel.Track("SiteMetrics", r)

		if err := b.siteMetrics(w, r); err != nil {
			var customErr *util.CustomError
			if errors.As(err, &customErr) {
				http.Error(
					w, customErr.Error(), customErr.Code,
				)
			} else {
				http.Error(
					w,
					"Internal Server Error",
					http.StatusInternalServerError,
				)
			}
			return
		}
	}
}

func (b *BlogService) siteMetrics(w http.ResponseWriter, r *http.Request) error {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		return util.CreateCustomError("", http.StatusNotFound)
	}
	blogidint, err := strconv.ParseInt(mux.Vars(r)["blogID"], 10, 32)
	if err != nil {
		return fmt.Errorf("cannot parse blog id: %w", err)
	}
	data, err := b.getSiteMetrics(int32(blogidint))
	if err != nil {
		return fmt.Errorf("error getting subscriber metrics: %w", err)
	}
	util.ExecTemplate(w, []string{"site_metrics.html", "posts.html"},
		util.PageInfo{
			Data: struct {
				Title    string
				UserInfo *session.UserInfo
				SiteData
				PostData []postdata
			}{
				Title:    "Dashboard",
				UserInfo: session.ConvertSessionToUserInfo(sesh),
				PostData: data,
			},
		},
		template.FuncMap{},
		logging.Logger(r),
	)
	return nil
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
				config.Config.Progstack.ServiceName,
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
				config.Config.Progstack.ServiceName,
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
