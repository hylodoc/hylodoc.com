package blog

import (
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/util"
)

type SiteData struct {
	CumulativeCounts template.JS
}

type PostData struct {
	Title     string
	Url       string
	Date      time.Time
	EmailData EmailData
}

type EmailData struct {
	Deliveries int
	Opens      int
	OpenRate   int
}

func (b *BlogService) SiteMetrics() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, _ := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if session == nil {
			http.Error(w, "", http.StatusNotFound)
			return
		}

		data, err := b.siteMetrics(w, r)
		if err != nil {
			log.Printf("error getting subscriber metrics: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		util.ExecTemplate(w, []string{"site_metrics.html", "posts.html"},
			util.PageInfo{
				Data: struct {
					Title   string
					Session *auth.Session
					SiteData
					PostData []PostData
				}{
					Title:    "Dashboard",
					Session:  session,
					PostData: data,
				},
			},
			template.FuncMap{},
		)
	}
}

func (b *BlogService) siteMetrics(w http.ResponseWriter, r *http.Request) ([]PostData, error) {
	return []PostData{
		PostData{
			Title: "#22 On the local-constant case",
			Url:   "xr0.localhost:7999",
			Date:  time.Now(),
			EmailData: EmailData{
				Deliveries: 3,
				Opens:      5,
				OpenRate:   40,
			},
		},
		PostData{
			Title: "#21 Refactoring with great zeal",
			Url:   "xr0.localhost:7999",
			Date:  time.Now(),
			EmailData: EmailData{
				Deliveries: 5,
				Opens:      7,
				OpenRate:   52,
			},
		},
	}, nil
}
