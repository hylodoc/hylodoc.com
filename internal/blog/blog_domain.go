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

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/dns"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/util"
)

type SubdomainRequest struct {
	Subdomain string `json:"subdomain"`
}

type SubdomainCheckResponse struct {
	Available bool   `json:"available"`
	Message   string `json:"message"`
}

func (b *BlogService) SubdomainCheck() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("SubdomainCheck handler...")

		b.mixpanel.Track("SubdomainCheck", r)

		available := true
		message := "Subdomain is available"
		if err := b.subdomainCheck(w, r); err != nil {
			var customErr *util.CustomError
			if errors.As(err, &customErr) {
				logger.Printf("Client error: %v\n", customErr)
				available = false
				message = customErr.Error()
			} else {
				logger.Printf("Internal Server Error: %v\n", err)
				http.Error(w, "", http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(SubdomainCheckResponse{
			Available: available,
			Message:   message,
		}); err != nil {
			logger.Printf("Error encoding reponse: %v\n", err)
			http.Error(
				w,
				"Error encoding reponse",
				http.StatusInternalServerError,
			)
			return
		}
	}
}

func (b *BlogService) subdomainCheck(w http.ResponseWriter, r *http.Request) error {
	var req SubdomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return err
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
	if err = validateSubdomain(req.Subdomain); err != nil {
		return err
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

func (b *BlogService) SubdomainSubmit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("SubdomainSubmit handler...")

		b.mixpanel.Track("SubdomainSubmit", r)

		if err := b.subdomainSubmit(w, r); err != nil {
			logger.Println("Error submiting subdomain")
			var customErr *util.CustomError
			if errors.As(err, &customErr) {
				logger.Printf("Custom error: %v\n", customErr)
				/* user error */
				w.WriteHeader(customErr.Code)
				response := map[string]string{"message": customErr.Error()}
				json.NewEncoder(w).Encode(response)
				return
			} else {
				logger.Printf("Internal Server Error: %v\n", err)
				/* generic error */
				w.WriteHeader(http.StatusInternalServerError)
				response := map[string]string{"message": "An unexpected error occurred"}
				json.NewEncoder(w).Encode(response)
				return
			}
		}
		/* success */
		w.WriteHeader(http.StatusOK)
		response := map[string]string{"message": "Subdomain successfully registered!"}
		json.NewEncoder(w).Encode(response)
	}
}

func (b *BlogService) subdomainSubmit(w http.ResponseWriter, r *http.Request) error {
	blogID, err := strconv.ParseInt(mux.Vars(r)["blogID"], 10, 32)
	if err != nil {
		return fmt.Errorf("cannot parse id: %w", err)
	}

	var req SubdomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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

	if err := b.store.UpdateSubdomainTx(
		context.TODO(), model.UpdateSubdomainTxParams{
			BlogID:    int32(blogID),
			Subdomain: sub,
		},
	); err != nil {
		return err
	}
	return nil
}

func (b *BlogService) DomainSubmit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("DomainSubmit handler...")

		b.mixpanel.Track("DomainSubmit", r)

		if err := b.domainSubmit(w, r); err != nil {
			logger.Println("Error submiting domain")
			var customErr *util.CustomError
			if errors.As(err, &customErr) {
				logger.Printf("Custom error: %v\n", customErr)
				/* user error */
				w.WriteHeader(customErr.Code)
				response := map[string]string{"message": customErr.Error()}
				json.NewEncoder(w).Encode(response)
				return
			} else {
				logger.Printf("Internal Server Error: %v\n", err)
				/* generic error */
				w.WriteHeader(http.StatusInternalServerError)
				response := map[string]string{"message": "An unexpected error occurred"}
				json.NewEncoder(w).Encode(response)
				return
			}
		}
	}
}

func (b *BlogService) domainSubmit(w http.ResponseWriter, r *http.Request) error {
	blogID, err := strconv.ParseInt(mux.Vars(r)["blogID"], 10, 32)
	if err != nil {
		return fmt.Errorf("cannot parse id: %w", err)
	}

	if r.Method != http.MethodPost {
		return util.CreateCustomError(
			"Invalid request method", http.StatusMethodNotAllowed,
		)
	}
	if err := r.ParseForm(); err != nil {
		return util.CreateCustomError(
			"Error parsing form", http.StatusBadRequest,
		)
	}

	if err := b.store.UpdateDomainByID(
		context.TODO(), model.UpdateDomainByIDParams{
			ID: int32(blogID),
			Domain: wrapNullString(
				strings.TrimSpace(
					strings.ToLower(r.FormValue("domain")),
				),
			),
		},
	); err != nil {
		return err
	}
	http.Redirect(
		w, r,
		fmt.Sprintf(
			"%s://%s/user/blogs/%d/config",
			config.Config.Progstack.Protocol,
			config.Config.Progstack.ServiceName,
			blogID,
		),
		http.StatusTemporaryRedirect,
	)
	return nil
}

func wrapNullString(domain string) sql.NullString {
	if domain == "" {
		return sql.NullString{}
	}
	return sql.NullString{Valid: true, String: domain}
}
