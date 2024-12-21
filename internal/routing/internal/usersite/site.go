package usersite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/xr0-org/progstack/internal/blog"
	"github.com/xr0-org/progstack/internal/dns"
	"github.com/xr0-org/progstack/internal/model"
)

type Site struct {
	_b model.Blog
}

var ErrIsService = errors.New("host is service name")
var ErrUnknownSubdomain = errors.New("unknown subdomain")

func GetSite(host string, s *model.Store) (*Site, error) {
	/* XXX: bit dodge but with local development we have subdomains like
	* http://<subdomain>.localhost:7999 which should also route
	* correctly so we split on both "." and ":" */
	parts := regexp.MustCompile(`[.:]`).Split(
		strings.ReplaceAll(host, "127.0.0.1", "localhost"),
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
			return nil, fmt.Errorf("no such site")
		}
		return nil, fmt.Errorf("blog get error: %w", err)
	}
	if !b.IsLive {
		return nil, fmt.Errorf("blog is offline")
	}
	return &Site{b}, nil
}

func (site *Site) RecordVisit(path string, store *model.Store) error {
	return store.RecordBlogVisit(
		context.TODO(),
		model.RecordBlogVisitParams{Url: path, Blog: site._b.ID},
	)
}

func (site *Site) GetBinding(path string, store *model.Store) (string, error) {
	gen, err := blog.GetFreshGeneration(site._b.ID, store)
	if err != nil {
		return "", fmt.Errorf("generation: %w", err)
	}
	binding, err := store.GetBinding(
		context.TODO(),
		model.GetBindingParams{Generation: gen, Url: path},
	)
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}
	return binding, nil
}

func (site *Site) RecordEmailClick(url *url.URL, store *model.Store) bool {
	values := url.Query()
	if !values.Has("subscriber") {
		return false
	}
	if err := recordemailclick(values.Get("subscriber"), store); err != nil {
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
