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
	data, err := b.getSiteMetrics(r)
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
	Title     string
	Url       template.URL
	Date      *time.Time
	Visits    int
	EmailData emaildata
}

type emaildata struct {
	Deliveries int
	Opens      int
	OpenRate   int
}

/* TODO: get from config */
var baseurl string = "localhost:7999"

func (b *BlogService) getSiteMetrics(r *http.Request) ([]postdata, error) {
	blogidint, err := strconv.ParseInt(mux.Vars(r)["blogID"], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("cannot parse blog id: %w", err)
	}
	blogid := int32(blogidint)
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
			/* XXX */
			fmt.Sprintf("http://%s.%s", blog.Subdomain, baseurl),
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
		data[i] = postdata{
			Title:  p.Title,
			Url:    template.URL(u),
			Date:   unwrapsqltime(p.PublishedAt),
			Visits: int(visits),
			EmailData: emaildata{
				Deliveries: 3,
				Opens:      5,
				OpenRate:   40,
			},
		}
	}
	return data, nil
}

func unwrapsqltime(time sql.NullTime) *time.Time {
	if time.Valid {
		return &time.Time
	}
	return nil
}
