package subdomain

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/xr0-org/progstack/internal/blog"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
)

type request struct {
	_subdomain string
	_url       url.URL
}

func parseRequest(r *http.Request) (*request, error) {
	/* XXX: bit dodge but with local development we have subdomains like
	* http://<subdomain>.localhost:7999 which should also route
	* correctly so we split on both "." and ":" */
	parts := regexp.MustCompile(`[.:]`).Split(
		strings.ReplaceAll(
			gethostorxforwardedhost(r), "127.0.0.1", "localhost",
		),
		-1,
	)
	if len(parts) < 1 {
		return nil, fmt.Errorf("dodge regex wrong part count")
	}
	return &request{parts[0], *r.URL}, nil
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

func (r *request) recordsitevisit(s *model.Store) error {
	b, err := s.GetBlogIDBySubdomain(context.TODO(), r._subdomain)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errNoSubdomain
		}
		return fmt.Errorf("cannot get blog id: %w", err)
	}
	return s.RecordBlogVisit(
		context.TODO(),
		model.RecordBlogVisitParams{Url: r._url.Path, Blog: b},
	)
}

func (r *request) getfilepath(s *model.Store) (string, error) {
	blogid, err := s.GetBlogIDBySubdomain(context.TODO(), r._subdomain)
	if err != nil {
		return "", fmt.Errorf("cannot get blog id: %w", err)
	}
	gen, err := blog.GetFreshGeneration(blogid, s)
	if err != nil {
		return "", fmt.Errorf("cannot get generation: %w", err)
	}
	path, err := s.GetBinding(
		context.TODO(),
		model.GetBindingParams{Generation: gen, Url: r._url.Path},
	)
	if err != nil {
		return "", fmt.Errorf("cannot get binding: %w", err)
	}
	return path, nil
}

func (r *request) recordemailclick(s *model.Store) bool {
	values := r._url.Query()
	if !values.Has("subscriber") {
		return false
	}
	if err := recordemailclick(values.Get("subscriber"), s); err != nil {
		/* TODO: emit metric */
		log.Println("emit metric:", err)
	}
	return true
}

func recordemailclick(rawtoken string, s *model.Store) error {
	token, err := uuid.Parse(rawtoken)
	if err != nil {
		return fmt.Errorf("cannot parse token: %w", err)
	}
	return s.SetSubscriberEmailClicked(context.TODO(), token)
}

func (r *request) redirecturl() string {
	u := r._url
	u.RawQuery = ""
	return u.String()
}
