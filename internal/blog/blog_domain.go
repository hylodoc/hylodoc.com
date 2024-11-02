package blog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"unicode"

	"github.com/gorilla/mux"
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
		log.Printf("check subdomain handler...")

		available := true
		message := "Subdomain is available"
		if err := b.subdomainCheck(w, r); err != nil {
			var customErr *util.CustomError
			if errors.As(err, &customErr) {
				log.Printf("client error: %v\n", customErr)
				available = false
				message = customErr.Error()
			} else {
				log.Printf("internal server error: %v\n", err)
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
			log.Printf("failed to encode response: %v", err)
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
			return
		}
	}
}

func (b *BlogService) subdomainCheck(w http.ResponseWriter, r *http.Request) error {
	var req SubdomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return err
	}
	exists, err := b.store.SubdomainExists(context.TODO(), req.Subdomain)
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
		log.Printf("submit subdomain handler...")

		/* XXX: metrics */

		if err := b.subdomainSubmit(w, r); err != nil {
			log.Println("error submiting subdomain")
			var customErr *util.CustomError
			if errors.As(err, &customErr) {
				log.Printf("user error: %v\n", customErr)
				/* user error */
				w.WriteHeader(customErr.Code)
				response := map[string]string{"message": customErr.Error()}
				json.NewEncoder(w).Encode(response)
				return
			} else {
				log.Printf("Internal Server Error: %v\n", err)
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
	blogID := mux.Vars(r)["blogID"]
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return err
	}

	var req SubdomainRequest
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		return err
	}
	if err = validateSubdomain(req.Subdomain); err != nil {
		return err
	}

	/* get existing blog */
	blog, err := b.store.GetBlogByID(context.TODO(), int32(intBlogID))
	if err != nil {
		return err
	}

	/* take current blog offline */
	if _, err := setBlogToOffline(blog, b.store); err != nil {
		return fmt.Errorf(
			"error taking subdomain `%s' offline: %w",
			blog.Subdomain, err,
		)
	}

	if err = b.store.UpdateSubdomainTx(context.TODO(), model.UpdateSubdomainTxParams{
		BlogID:    int32(intBlogID),
		Subdomain: req.Subdomain,
	}); err != nil {
		return err
	}

	/* bring blog online */
	if _, err := setBlogToLive(&blog, b.store); err != nil {
		return fmt.Errorf(
			"error bringing subdomain `%s' online: %w",
			blog.Subdomain, err,
		)
	}

	return nil
}
