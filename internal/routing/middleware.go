package routing

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/routing/internal/usersite"
)

type RoutingService struct {
	store *model.Store
}

func NewRoutingService(s *model.Store) *RoutingService {
	return &RoutingService{store: s}
}

func (s *RoutingService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		if err := s.tryRenderUsersite(w, r); err != nil {
			if errors.Is(err, usersite.ErrIsService) {
				next.ServeHTTP(w, r)
			} else {
				logger.Println("unknown host error:", err)
				/* TODO: unknown host pages */
				if errors.Is(err, usersite.ErrUnknownSubdomain) {
					http.Error(
						w,
						"unclaimed subdomain",
						http.StatusNotFound,
					)
				} else {
					http.Error(
						w,
						"unknown domain",
						http.StatusNotFound,
					)
				}
			}
		}
	})
}

func (s *RoutingService) tryRenderUsersite(
	w http.ResponseWriter, r *http.Request,
) error {
	site, err := usersite.GetSite(r.Host, s.store)
	if err != nil {
		return fmt.Errorf("get site: %w", err)
	}
	/* site visit is only recorded after checking for email token because
	 * the redirect would cause two visits to be recorded. */
	if site.RecordEmailClick(r.URL, s.store) {
		http.Redirect(
			w, r,
			stripEmailToken(r.URL),
			http.StatusPermanentRedirect,
		)
		return nil
	}
	if err := site.RecordVisit(r.URL.Path, s.store); err != nil {
		return fmt.Errorf("record visit: %w", err)
	}
	filepath, err := site.GetBinding(r.URL.Path, s.store)
	if err != nil {
		return fmt.Errorf("get filepath: %w", err)
	}
	http.ServeFile(w, r, filepath)
	return nil
}

func stripEmailToken(url *url.URL) string {
	u := *url
	u.RawQuery = ""
	return u.String()
}
