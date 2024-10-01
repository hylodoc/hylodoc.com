package blog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"unicode"

	"github.com/gorilla/mux"
	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/util"
)

const (
	usersiteTemplatePath = "usersite_template" /* XXX: temporary this will all be generated */
)

type BlogService struct {
	store        *model.Store
	resendClient *resend.Client
}

func NewBlogService(store *model.Store, resendClient *resend.Client) *BlogService {
	return &BlogService{store: store, resendClient: resendClient}
}

func (b *BlogService) SubscribeToBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("subscribe to blog handler...")

		if err := b.subscribeToBlog(w, r); err != nil {
			log.Printf("error subscribing to blog: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

type SubscribeRequest struct {
	Email string `json:"email"`
}

func (sr *SubscribeRequest) validate() error {
	if sr.Email == "" {
		return fmt.Errorf("email is required")
	}
	return nil
}

func (b *BlogService) subscribeToBlog(w http.ResponseWriter, r *http.Request) error {
	/* extract BlogID from path */
	vars := mux.Vars(r)
	blogID := vars["blogID"]

	/* parse the request body to get subscriber email */
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("error reading request body: %w", err)
	}
	defer r.Body.Close()

	var req SubscribeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("error unmarshaling request: %w", err)
	}
	if err = req.validate(); err != nil {
		return fmt.Errorf("error invalid request body: %w", err)
	}

	/* XXX: validate email format */

	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return fmt.Errorf("error converting string path var to blogID: %w", err)
	}

	unsubscribeToken, err := auth.GenerateToken()
	if err != nil {
		return fmt.Errorf("error generating unsubscribeToken: %w", err)
	}

	log.Printf("subscribing email `%s' to blog with id: `%d'", req.Email, intBlogID)
	/* first check if exists */

	err = b.store.CreateSubscriberTx(context.TODO(), model.CreateSubscriberTxParams{
		BlogID:           int32(intBlogID),
		Email:            req.Email,
		UnsubscribeToken: unsubscribeToken,
	})
	if err != nil {
		return fmt.Errorf("error writing subscriber for blog to db: %w", err)
	}
	return nil
}

func (b *BlogService) UnsubscribeFromBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("unsubscribe from blog handler...")
		if err := b.unsubscribeFromBlog(w, r); err != nil {
			log.Printf("error in unsubscribeFromBlog handler: %w", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

type UnsubscribeRequest struct {
	Token string `json:"token"`
}

func (ur *UnsubscribeRequest) validate() error {
	if ur.Token == "" {
		return fmt.Errorf("token is required")
	}
	return nil
}

func (b *BlogService) unsubscribeFromBlog(w http.ResponseWriter, r *http.Request) error {
	/* extract BlogID from path */
	vars := mux.Vars(r)
	blogID := vars["blogID"]

	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return fmt.Errorf("error converting string path var to blogID: %w", err)
	}
	token := r.URL.Query().Get("token")
	err = b.store.DeleteSubscriberForBlog(context.TODO(), model.DeleteSubscriberForBlogParams{
		BlogID:           int32(intBlogID),
		UnsubscribeToken: token,
	})
	if err != nil {
		return fmt.Errorf("error writing subscriber for blog to db: %w", err)
	}
	return nil
}

/* Blog configuration page */

func (b *BlogService) Config() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("blog config handler...")

		session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
		if !ok {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}
		blogID := mux.Vars(r)["blogID"]
		intBlogID, err := strconv.ParseInt(blogID, 10, 32)
		if err != nil {
			log.Println("error converting string path var to blogID: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}

		util.ExecTemplate(w, []string{"config.html"},
			util.PageInfo{
				Data: struct {
					Title   string
					Session *auth.Session
					ID      int32
				}{
					Title:   "Blog Setup",
					Session: session,
					ID:      int32(intBlogID),
				},
			},
		)
	}
}

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

		err := b.subdomainCheck(w, r)
		available := true
		message := "subdomain available"
		if err != nil {
			userErr, ok := err.(util.UserError)
			if !ok {
				/* internal error */
				http.Error(w, "", http.StatusInternalServerError)
				return
			}
			log.Printf("user error: %v\n", userErr)
			available = false
			message = userErr.Message
		}
		/* build response object */

		w.WriteHeader(http.StatusOK)
		err = json.NewEncoder(w).Encode(SubdomainCheckResponse{
			Available: available,
			Message:   message,
		})
		if err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
	}
}

func (b *BlogService) subdomainCheck(w http.ResponseWriter, r *http.Request) error {
	var req SubdomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return err
	}
	exists, err := b.store.SubdomainExists(context.TODO(), sql.NullString{
		Valid:  true,
		String: req.Subdomain,
	})
	if err != nil {
		return fmt.Errorf("error checking for subdomain in db: %w", err)
	}
	if exists {
		return util.UserError{
			Message: "subdomain already exists",
		}
	}
	/* check if valid subdomain */
	if err = validateSubdomain(req.Subdomain); err != nil {
		return err
	}

	return nil
}

func validateSubdomain(subdomain string) error {
	if len(subdomain) < 1 || len(subdomain) > 63 {
		return util.UserError{
			Message: "Subdomain must be between 1 and 63 characters long",
		}
	}
	for _, r := range subdomain {
		if unicode.IsSpace(r) {
			return util.UserError{
				Message: "Subdomain cannot contain spaces.",
			}
		}
	}
	previousChar := ' ' /* start with a space to avoid consecutive check on the first character */
	for _, r := range subdomain {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-') {
			return util.UserError{
				Message: "Subdomain can only contain letters, numbers, and hyphens.",
			}
		}

		/* check for consecutive hyphens */
		if r == '-' && previousChar == '-' {
			return util.UserError{
				Message: "Subdomain cannot contain consecutive hyphens.",
			}
		}
		previousChar = r
	}
	/* check that it does not start or end with a hyphen */
	if subdomain[0] == '-' || subdomain[len(subdomain)-1] == '-' {
		return util.UserError{
			Message: "Subdomain cannot start or end with a hyphen.",
		}
	}
	return nil
}

func (b *BlogService) SubdomainSubmit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("submit subdomain handler...")

		/* XXX: metrics */

		if err := b.subdomainSubmit(w, r); err != nil {
			log.Println("error submiting subdomain")
			if userErr, ok := err.(util.UserError); ok {
				log.Printf("user error: %v\n", userErr)
				/* user error */
				w.WriteHeader(userErr.Code)
				response := map[string]string{"message": userErr.Message}
				json.NewEncoder(w).Encode(response)
				return
			}
			log.Printf("server error: %v\n", err)
			/* generic error */
			w.WriteHeader(http.StatusInternalServerError)
			response := map[string]string{"message": "An unexpected error occurred"}
			json.NewEncoder(w).Encode(response)
			return
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
	if err = b.store.CreateSubdomainTx(context.TODO(), model.CreateSubdomainTxParams{
		BlogID:    int32(intBlogID),
		Subdomain: req.Subdomain,
	}); err != nil {
		return err
	}
	return nil
}

/* Launch Blog */

type LaunchBlogParams struct {
	GhRepoFullName string
	Subdomain      string
}

func (b *BlogService) LaunchBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("launch blog handler...")

		if err := b.launchBlog(w, r); err != nil {
			log.Println("error submiting subdomain")
			if userErr, ok := err.(util.UserError); ok {
				log.Printf("user error: %v\n", userErr)
				/* user error */
				w.WriteHeader(userErr.Code)
				response := map[string]string{"message": userErr.Message}
				json.NewEncoder(w).Encode(response)
				return
			}
			log.Printf("server error: %v\n", err)
			/* generic error */
			w.WriteHeader(http.StatusInternalServerError)
			response := map[string]string{"message": "An unexpected error occurred"}
			json.NewEncoder(w).Encode(response)
			return
		}
		/* success */
		w.WriteHeader(http.StatusOK)
		response := map[string]string{"message": "Blog successfully launched!"}
		json.NewEncoder(w).Encode(response)
	}
}

func (b *BlogService) launchBlog(w http.ResponseWriter, r *http.Request) error {
	blogID := mux.Vars(r)["blogID"]
	intBlogID, err := strconv.ParseInt(blogID, 10, 32)
	if err != nil {
		return fmt.Errorf("error converting string path var to blogID: %w", err)
	}

	blog, err := b.store.GetBlogByID(context.TODO(), int32(intBlogID))
	if err != nil {
		return fmt.Errorf("error getting blog: %w", err)
	}

	ghRepoFullName := blog.GhFullName
	log.Printf("launching user website at `%s'...\n", ghRepoFullName)

	/* XXX: generate website content and server from repository path */
	/* repositoryPath is like: /repositories/<gh_user>/<gh_repository_name> on disk */
	repositoryPath := fmt.Sprintf("%s/%s", config.Config.Progstack.RepositoriesPath, ghRepoFullName)
	/* for now we just check it exists */
	log.Printf("repositoryPath: `%s'\n", repositoryPath)
	_, err = os.Stat(repositoryPath)
	if os.IsNotExist(err) {
		log.Printf("repositoryPath does `%s' does not exist on disk: %v\n", repositoryPath, err)
		return fmt.Errorf("repository does not exist on disk: %w", err)
	}

	/* XXX: for now before we have generation we just copy a template site
	* and pretend it's generated */
	websitePath := fmt.Sprintf("%s/%s", config.Config.Progstack.WebsitesPath, blog.Subdomain)
	if err := copyDir(usersiteTemplatePath, websitePath); err != nil {
		log.Printf("error lcopying template from src `%s' to dst `%s': %v\n", repositoryPath, websitePath, err)
		return fmt.Errorf("error copying template to site destination: %w", err)
	}
	return nil
}

func copyDir(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dstDir, os.ModePerm); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())
		if entry.IsDir() {
			/* if dir recurse */
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			/* if file copy */
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
