package sites

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/xr0-org/progstack-ssg/pkg/ssg"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
)

const (
	usersiteTemplatePath = "usersite_template" /* XXX: temporary this will all be generated */
)

type UserWebsiteMiddleware struct {
	store *model.Store
}

func NewUserWebsiteMiddleware(s *model.Store) *UserWebsiteMiddleware {
	return &UserWebsiteMiddleware{
		store: s,
	}
}

func (uwm *UserWebsiteMiddleware) RouteToSubdomains(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("running subdomain middleware...")

		log.Println("received request for: ", r.URL)

		/* extract subdomain */
		host := r.Header.Get("X-Forwarded-Host")
		log.Printf("X-Forwarded-Host: %s\n", host)
		if host == "" {
			host = r.Host // Fallback to the Host header
		}
		log.Printf("Host: %s\n", host)

		/* needed for the following splitting to work on localhost */
		host = strings.ReplaceAll(host, "127.0.0.1", "localhost")

		/* XXX: bit dodge but with local development we have subdomains like
		* http://<subdomain>.localhost:7999 whic should also route
		* correctly so we split on both "." and ":" */
		re := regexp.MustCompile(`[.:]`)
		parts := re.Split(host, -1)
		if len(parts) > 2 {
			subdomain := parts[0]
			log.Printf("subdomain: %s\n", subdomain)
			/* path to generated site */
			userSitePath := fmt.Sprintf("%s/%s/", config.Config.Progstack.WebsitesPath, subdomain)
			log.Printf("userSitePath: %s\n", userSitePath)

			/* check if file exists */
			filePath := filepath.Join(userSitePath, r.URL.Path)
			if r.URL.Path == "/" {
				/* no specific file requested */
				filePath = filepath.Join(userSitePath, "index.html")
			}
			log.Printf("filePath: %s\n", filePath)

			http.ServeFile(w, r, filePath)
			return
		}

		/* not a subdomain next middleware */
		next.ServeHTTP(w, r)
	})
}

type LaunchUserBlogParams struct {
	GhRepoFullName string
	Subdomain      string
}

func LaunchUserBlog(params LaunchUserBlogParams) error {
	repo := filepath.Join(
		config.Config.Progstack.RepositoriesPath,
		params.GhRepoFullName,
	)
	if _, err := os.Stat(repo); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("repository does not exist on disk: %w", err)
		}
		return err
	}
	site := filepath.Join(
		config.Config.Progstack.WebsitesPath,
		params.Subdomain,
	)
	return ssg.Generate(repo, site, "")
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

/* This is how user sites can interact with the main application */

type UserWebsiteService struct {
	Subdomain string
	Folder    string
	Store     *model.Store
}

func (uw *UserWebsiteService) GetComments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Println("handling GetComments request...")

		assert.Printf(false, "not implemented")
	}
}
