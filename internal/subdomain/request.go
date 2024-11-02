package subdomain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
)

type request struct {
	_subdomain, _url string
}

func parseRequest(r *http.Request) (*request, error) {
	/* XXX: bit dodge but with local development we have subdomains like
	* http://<subdomain>.localhost:7999 whic should also route
	* correctly so we split on both "." and ":" */
	re := regexp.MustCompile(`[.:]`)
	parts := re.Split(
		strings.ReplaceAll(
			gethostorxforwardedhost(r), "127.0.0.1", "localhost",
		),
		-1,
	)
	if len(parts) < 1 {
		return nil, fmt.Errorf("dodge regex wrong part count")
	}
	return &request{parts[0], r.URL.Path}, nil
}

func gethostorxforwardedhost(r *http.Request) string {
	host := r.Header.Get("X-Forwarded-Host")
	logging.Logger(r).Printf("X-Forwarded-Host: %s\n", host)
	if host == "" {
		return r.Host // Fallback to the Host header
	}
	return host
}

var errNoSubdomain = errors.New("no such subdomain")

func (r *request) recordvisit(s *model.Store) error {
	blogid, err := s.GetBlogIDBySubdomain(context.TODO(), r._subdomain)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errNoSubdomain
		}
		return fmt.Errorf("cannot get blog id: %w", err)
	}
	return s.RecordBlogVisit(
		context.TODO(),
		model.RecordBlogVisitParams{Url: r._url, Blog: blogid},
	)
}

func (r *request) getfilepath(s *model.Store) (string, error) {
	gen, err := s.GetLastGenerationBySubdomain(context.TODO(), r._subdomain)
	if err != nil {
		return "", fmt.Errorf("cannot get generation: %w", err)
	}
	path, err := s.GetBinding(
		context.TODO(),
		model.GetBindingParams{Generation: gen, Url: r._url},
	)
	if err != nil {
		return "", fmt.Errorf("cannot get binding: %w", err)
	}
	return path, nil
}
