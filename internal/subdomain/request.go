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
	"github.com/xr0-org/progstack/internal/dns"
	"github.com/xr0-org/progstack/internal/model"
)

type request struct {
	_b   model.Blog
	_url url.URL
}

var errNoSubdomain = errors.New("no such subdomain")

func parseRequest(r *http.Request, s *model.Store) (*request, error) {
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
	sub, err := dns.ParseSubdomain(parts[0])
	if err != nil {
		return nil, fmt.Errorf("subdomain: %w", err)
	}
	b, err := s.GetBlogBySubdomain(context.TODO(), sub)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%q: %w", parts, errNoSubdomain)
		}
		return nil, fmt.Errorf("blog get error: %w", err)
	}
	if !b.IsLive {
		return nil, fmt.Errorf("blog is offline")
	}
	return &request{b, *r.URL}, nil
}

func gethostorxforwardedhost(r *http.Request) string {
	if host := r.Header.Get("X-Forwarded-Host"); host != "" {
		return host
	}
	return r.Host // Fallback to the Host header
}

func (r *request) recordsitevisit(s *model.Store) error {
	return s.RecordBlogVisit(
		context.TODO(),
		model.RecordBlogVisitParams{Url: r._url.Path, Blog: r._b.ID},
	)
}

func (r *request) getfilepath(s *model.Store) (string, error) {
	gen, err := blog.GetFreshGeneration(r._b.ID, s)
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
