package blog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/lib/pq"
	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/authz"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/dns"
	"github.com/xr0-org/progstack/internal/model"
)

type SubdomainRequest struct {
	Subdomain string `json:"subdomain"`
}

func (b *BlogService) SubdomainCheck(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("SubdomainCheck handler...")

	r.MixpanelTrack("SubdomainCheck")

	type checkresp struct {
		Available bool   `json:"available"`
		Message   string `json:"message"`
	}
	if err := b.subdomainCheck(r); err != nil {
		var customErr *customError
		if errors.As(err, &customErr) {
			return response.NewJson(
				checkresp{false, customErr.Error()},
			)
		}
		return nil, fmt.Errorf("check: %w", err)
	}
	return response.NewJson(checkresp{true, "Subdomain is available"})
}

func (b *BlogService) subdomainCheck(r request.Request) error {
	var req SubdomainRequest
	body, err := r.ReadBody()
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	sub, err := dns.ParseSubdomain(req.Subdomain)
	if err != nil {
		var parseErr dns.ParseUserError
		if errors.As(err, &parseErr) {
			return createCustomError(
				parseErr.Error(),
				http.StatusBadRequest,
			)
		}
		return fmt.Errorf("subdomain: %w", err)
	}
	taken, err := b.store.SubdomainIsTaken(context.TODO(), sub)
	if err != nil {
		return fmt.Errorf("subdomain is taken: %w", err)
	}
	if taken {
		return createCustomError(
			"subdomain already exists",
			http.StatusBadRequest,
		)
	}
	return nil
}

func (b *BlogService) SubdomainSubmit(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("SubdomainSubmit handler...")

	r.MixpanelTrack("SubdomainSubmit")

	type submitresp struct {
		Message string `json:"message"`
	}
	if err := b.subdomainSubmit(r); err != nil {
		sesh.Println("Error submiting subdomain")
		var customErr *customError
		if errors.As(err, &customErr) {
			return response.NewJson(&submitresp{customErr.Error()})
		} else {
			return nil, fmt.Errorf("subdomain submit: %w", err)
		}
	}
	return response.NewJson(&submitresp{"Subdomain successfully registered!"})
}

func (b *BlogService) subdomainSubmit(r request.Request) error {
	var req SubdomainRequest
	body, err := r.ReadBody()
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("cannot decode request: %w", err)
	}

	sub, err := dns.ParseSubdomain(req.Subdomain)
	if err != nil {
		var parseErr dns.ParseUserError
		if errors.As(err, &parseErr) {
			return createCustomError(
				parseErr.Error(),
				http.StatusBadRequest,
			)
		}
		return fmt.Errorf("subdomain: %w", err)
	}

	blogID, ok := r.GetRouteVar("blogID")
	if !ok {
		return fmt.Errorf("no blogID")
	}
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return fmt.Errorf("parse blogID: %w", err)
	}
	if err := b.store.UpdateBlogSubdomainByID(
		context.TODO(), model.UpdateBlogSubdomainByIDParams{
			ID:        int32(intBlogID),
			Subdomain: sub,
		},
	); err != nil {
		if isUniqueSubdomainViolation(err) {
			return createCustomError(
				"subdomain already exists",
				http.StatusBadRequest,
			)
		}
		return err
	}
	return nil
}

func isUniqueSubdomainViolation(err error) bool {
	var pqerr *pq.Error
	return errors.As(err, &pqerr) &&
		pqerr.Code.Name() == "unique_violation" &&
		pqerr.Constraint == "unique_blog_subdomain"
}

func (b *BlogService) DomainSubmit(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("DomainSubmit handler...")

	r.MixpanelTrack("DomainSubmit")

	userID, err := sesh.GetUserID()
	if err != nil {
		return nil, fmt.Errorf("get user id: %w", err)
	}
	has, err := authz.HasAnalyticsCustomDomainsImagesEmails(b.store, userID)
	if err != nil {
		return nil, fmt.Errorf("has analytics et al: %w", err)
	}
	if !has {
		return nil, authz.SubscriptionError
	}

	blogIDRaw, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, createCustomError("", http.StatusNotFound)
	}
	blogID, err := strconv.ParseInt(blogIDRaw, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse blogID: %w", err)
	}

	domain, err := r.GetPostFormValue("domain")
	if err != nil {
		r.Session().Printf("cannot get post form value: %v\n", err)
		return nil, createCustomError(
			"Error parsing form", http.StatusBadRequest,
		)
	}

	if err := b.store.UpdateBlogDomainByID(
		context.TODO(), model.UpdateBlogDomainByIDParams{
			ID: int32(blogID),
			Domain: wrapNullString(
				strings.TrimSpace(strings.ToLower(domain)),
			),
		},
	); err != nil {
		return nil, fmt.Errorf("update domain: %w", err)
	}
	return response.NewRedirect(
		fmt.Sprintf(
			"%s://%s/user/blogs/%d/config",
			config.Config.Progstack.Protocol,
			config.Config.Progstack.RootDomain,
			blogID,
		),
		http.StatusTemporaryRedirect,
	), nil
}

func wrapNullString(domain string) sql.NullString {
	if domain == "" {
		return sql.NullString{}
	}
	return sql.NullString{Valid: true, String: domain}
}
