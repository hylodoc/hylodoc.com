package usersite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/blog"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/dns"
	"github.com/xr0-org/progstack/internal/model"
)

type Site struct {
	blogID string
}

var ErrIsService = errors.New("host is service name")
var ErrPageNotFound = errors.New("page not found")
var ErrUnknownSubdomain = errors.New("unknown subdomain")
var ErrUnknownDomain = errors.New("unknown domain")

func GetSite(host string, s *model.Store) (*Site, error) {
	if host == config.Config.Progstack.RootDomain {
		return nil, ErrIsService
	}
	blog, err := getBlog(host, s)
	if err != nil {
		return nil, fmt.Errorf("get blog: %w", err)
	}
	return &Site{blog}, nil
}

func getBlog(host string, s *model.Store) (string, error) {
	/* check for subdomain first because it's the more common case */
	blogID, err := getBlogBySubdomain(host, s)
	if err == nil {
		return blogID, nil
	}
	if !errors.Is(err, errNotSubdomainForm) {
		return "", fmt.Errorf("subdomain: %w", err)
	}
	assert.Assert(errors.Is(err, errNotSubdomainForm))

	blog, err := s.GetBlogByDomain(context.TODO(), host)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrUnknownDomain
		}
		return "", fmt.Errorf("domain: %w", err)
	}
	return blog.ID, nil
}

var errNotSubdomainForm = errors.New("not subdomain form")

func getBlogBySubdomain(host string, s *model.Store) (string, error) {
	/* `.hylodoc.com' (dot followed by service name) must follow host */
	subdomain, found := strings.CutSuffix(
		host,
		fmt.Sprintf(".%s", config.Config.Progstack.RootDomain),
	)
	if !found {
		return "", errNotSubdomainForm
	}
	sub, err := dns.ParseSubdomain(subdomain)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	blog, err := s.GetBlogBySubdomain(context.TODO(), sub)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrUnknownSubdomain
		}
		return "", fmt.Errorf("query error: %w", err)
	}
	return blog.ID, nil
}

func (site *Site) RecordVisit(path string, store *model.Store) error {
	return store.RecordBlogVisit(
		context.TODO(),
		model.RecordBlogVisitParams{Url: path, Blog: site.blogID},
	)
}

func (site *Site) GetBinding(path string, store *model.Store) (string, error) {
	gen, err := blog.GetFreshGeneration(site.blogID, store)
	if err != nil {
		return "", fmt.Errorf("generation: %w", err)
	}
	binding, err := store.GetBinding(
		context.TODO(),
		model.GetBindingParams{Generation: gen, Url: path},
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrPageNotFound
		}
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
