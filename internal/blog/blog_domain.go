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
	"unicode"

	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/authz"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/dns"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/util"
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
		var customErr *util.CustomError
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
			return util.CreateCustomError(
				parseErr.Error(),
				http.StatusBadRequest,
			)
		}
		return fmt.Errorf("subdomain: %w", err)
	}
	exists, err := b.store.SubdomainExists(context.TODO(), sub)
	if err != nil {
		return fmt.Errorf("error checking for subdomain in db: %w", err)
	}
	if exists {
		return util.CreateCustomError(
			"subdomain already exists",
			http.StatusBadRequest,
		)
	}
	return nil
}

func validateSubdomain(subdomain string) error {
	if len(subdomain) < 1 || len(subdomain) > 63 {
		return util.CreateCustomError(
			"Subdomain must be between 1 and 63 characters long",
			http.StatusBadRequest,
		)
	}
	for _, r := range subdomain {
		if unicode.IsSpace(r) {
			return util.CreateCustomError(
				"Subdomain cannot contain spaces.",
				http.StatusBadRequest,
			)
		}
	}
	previousChar := ' ' /* start with a space to avoid consecutive check on the first character */
	for _, r := range subdomain {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-') {
			return util.CreateCustomError(
				"Subdomain can only contain letters, numbers, and hyphens.",
				http.StatusBadRequest,
			)
		}
		/* check for consecutive hyphens */
		if r == '-' && previousChar == '-' {
			return util.CreateCustomError(
				"Subdomain cannot contain consecutive hyphens.",
				http.StatusBadRequest,
			)
		}
		previousChar = r
	}
	/* check that it does not start or end with a hyphen */
	if subdomain[0] == '-' || subdomain[len(subdomain)-1] == '-' {
		return util.CreateCustomError(
			"Subdomain cannot start or end with a hyphen.",
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
		var customErr *util.CustomError
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
			return util.CreateCustomError(
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
	if err := b.store.UpdateSubdomainTx(
		context.TODO(), model.UpdateSubdomainTxParams{
			BlogID:    int32(intBlogID),
			Subdomain: sub,
		},
	); err != nil {
		return err
	}
	return nil
}

func (b *BlogService) DomainSubmit(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("DomainSubmit handler...")

	r.MixpanelTrack("DomainSubmit")

	if err := authz.CanConfigureCustomDomain(
		b.store, r.Session(),
	); err != nil {
		return nil, fmt.Errorf("can configure custom domain: %w", err)
	}

	blogIDRaw, ok := r.GetRouteVar("blogID")
	if !ok {
		return nil, util.CreateCustomError("", http.StatusNotFound)
	}
	blogID, err := strconv.ParseInt(blogIDRaw, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse blogID: %w", err)
	}

	domain, err := r.GetPostFormValue("domain")
	if err != nil {
		r.Session().Printf("cannot get post form value: %v\n", err)
		return nil, util.CreateCustomError(
			"Error parsing form", http.StatusBadRequest,
		)
	}

	if err := b.store.UpdateDomainByID(
		context.TODO(), model.UpdateDomainByIDParams{
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
			config.Config.Progstack.ServiceName,
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
