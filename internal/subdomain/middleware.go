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
		logger.Println("running subdomain middleware...")
		logger.Println("received request for: ", r.URL)
		if err := ss.middleware(w, r); err != nil {
			logger.Println("no subdomain found: ")
			if !errors.Is(err, errNoSubdomain) {
				/* TODO: escalate worse error */
				logger.Println("critical subdomain error:", err)
			}
			logger.Println("no subdomain found:", err)
			next.ServeHTTP(w, r)
			return
		}
	})
}

func (ss *SubdomainService) middleware(
	w http.ResponseWriter, r *http.Request,
) error {
	req, err := parseRequest(r)
	if err != nil {
		return fmt.Errorf("cannot parse request: %w", err)
	}
	if err := req.recordvisit(ss.store); err != nil {
		return fmt.Errorf("cannot record visit: %w", err)
	}
	filepath, err := req.getfilepath(ss.store)
	if err != nil {
		return fmt.Errorf("cannot get filepath: %w", err)
	}
	logging.Logger(r).Println("filepath", filepath)
	http.ServeFile(w, r, filepath)
	return nil
}
