package sites

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/xr0-org/progstack-ssg/pkg/ssg"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
)

const (
	usersiteTemplatePath = "usersite_template" /* XXX: temporary this will all be generated */
)

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
