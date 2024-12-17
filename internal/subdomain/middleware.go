package subdomain

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
)

type SubdomainService struct {
	store *model.Store
}

func NewSubdomainService(s *model.Store) *SubdomainService {
	return &SubdomainService{store: s}
}

func (ss *SubdomainService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		if err := ss.middleware(w, r); err != nil {
			if !errors.Is(err, errNoSubdomain) {
				logger.Println("subdomain error:", err)
			}
			next.ServeHTTP(w, r)
			return
		}
	})
}

func (ss *SubdomainService) middleware(
	w http.ResponseWriter, r *http.Request,
) error {
	req, err := parseRequest(r, ss.store)
	if err != nil {
		return fmt.Errorf("cannot parse request: %w", err)
	}
	/* site visit is only recorded after checking for email token because
	 * the redirect would cause two visits to be recorded. */
	if req.recordemailclick(ss.store) {
		/* TODO: make permanent redirect */
		http.Redirect(
			w, r, req.redirecturl(), http.StatusTemporaryRedirect,
		)
		return nil
	}
	if err := req.recordsitevisit(ss.store); err != nil {
		return fmt.Errorf("cannot record visit: %w", err)
	}
	filepath, err := req.getfilepath(ss.store)
	if err != nil {
		return fmt.Errorf("cannot get filepath: %w", err)
	}
	http.ServeFile(w, r, filepath)
	return nil
}
