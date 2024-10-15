package blog

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/util"
)

const maxFileSize = 10 * 1024 * 1024 /* Limit file size to 10MB */

type CreateBlogResponse struct {
	Url     string `json:"url"`
	Message string `json:"message"`
}

func (b *BlogService) CreateRepositoryBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("create blog handler...")

		message := "Successfully created repository-based blog!"
		url, err := b.createRepositoryBlog(w, r)
		if err != nil {
			userErr, ok := err.(util.UserError)
			if !ok {
				log.Printf("Internal Server Error: %v\n", err)
				http.Error(w, "", http.StatusInternalServerError)
				return
			}
			log.Printf("Client Error: %v\n", userErr)
			http.Error(w, userErr.Error(), http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(CreateBlogResponse{
			Url:     url,
			Message: message,
		}); err != nil {
			log.Printf("failed to encode response: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}
}

type CreateRepositoryBlogRequest struct {
	Subdomain    string `json:"subdomain"`
	RepositoryID string `json:"repository_id"`
	TestBranch   string `json:"test_branch"`
	LiveBranch   string `json:"live_branch"`
	Flow         string `json:"flow"`
}

func (b *BlogService) createRepositoryBlog(w http.ResponseWriter, r *http.Request) (string, error) {
	session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
	if !ok {
		return "", fmt.Errorf("user not found")
	}

	var req CreateRepositoryBlogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("could not decode body: %v", err)
		return "", util.UserError{
			Message: "error decoding request body",
			Code:    http.StatusBadRequest,
		}
	}
	fmt.Printf("req: %v", req)

	intRepoID, err := strconv.ParseInt(req.RepositoryID, 10, 64)
	if err != nil {
		return "", fmt.Errorf("could not convert repositoryID `%s' to int64: %w", req.RepositoryID, err)
	}

	repo, err := b.store.GetRepositoryByGhRepositoryID(context.TODO(), intRepoID)
	if err != nil {
		return "", fmt.Errorf("could not get repository for ghRepoId `%s': %w", intRepoID, err)
	}

	blog, err := b.store.CreateBlog(context.TODO(), model.CreateBlogParams{
		UserID: session.UserID,
		GhRepositoryID: sql.NullInt64{
			Valid: true,
			Int64: intRepoID,
		},
		GhUrl: sql.NullString{
			Valid:  true,
			String: buildRepositoryUrl(repo.FullName),
		},
		RepositoryPath: buildRepositoryPath(session.UserID, repo.FullName),
		Subdomain:      req.Subdomain,
		TestBranch: sql.NullString{
			Valid:  true,
			String: req.TestBranch,
		},
		LiveBranch: sql.NullString{
			Valid:  true,
			String: req.LiveBranch,
		},
		FromAddress: config.Config.Progstack.FromEmail,
		BlogType:    model.BlogTypeRepository,
	})
	if err != nil {
		return "", fmt.Errorf("could not create blog: %w", err)
	}

	/* add first user as subscriber */
	if err = b.store.CreateSubscriberTx(context.TODO(), model.CreateSubscriberTxParams{
		BlogID: blog.ID,
		Email:  session.Email,
	}); err != nil {
		return "", fmt.Errorf("error creating first subscriber: %w", err)
	}

	/* take blog live  */
	_, err = setBlogToLive(blog, b.store)
	if err != nil {
		return "", fmt.Errorf("error setting blog to live: %w", err)
	}
	return buildDomainUrl(blog.Subdomain), nil
}

func buildRepositoryUrl(fullName string) string {
	return fmt.Sprintf(
		"https://github.com/%s/",
		fullName,
	)
}

func buildRepositoryPath(userID int32, repoFullName string) string {
	userIDString := strconv.FormatInt(int64(userID), 10)
	return filepath.Join(
		config.Config.Progstack.RepositoriesPath,
		userIDString,
		repoFullName,
	)
}

func (b *BlogService) CreateFolderBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("create folder blog handler...")

		message := "Successfully created folder-based blog."
		url, err := b.createFolderBlog(w, r)
		if err != nil {
			userErr, ok := err.(util.UserError)
			if !ok {
				/* internal error */
				log.Printf("internal error: %v\n", err)
				http.Error(w, "", http.StatusInternalServerError)
			}
			log.Printf("client error: %v\n", userErr)
			message = userErr.Message
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		if err = json.NewEncoder(w).Encode(CreateBlogResponse{
			Url:     url,
			Message: message,
		}); err != nil {
			http.Error(w, "failed to encode repsonse", http.StatusInternalServerError)
			return
		}
	}
}

func (b *BlogService) createFolderBlog(w http.ResponseWriter, r *http.Request) (string, error) {
	session, ok := r.Context().Value(auth.CtxSessionKey).(*auth.Session)
	if !ok {
		log.Println("user not found")
		return "", util.UserError{
			Message: "",
			Code:    http.StatusNotFound,
		}
	}

	/* XXX: Add subscription based file size limits */
	err := r.ParseMultipartForm(maxFileSize) /* 10MB limit */
	if err != nil {
		log.Printf("fiile too large: %v\n", err)
		return "", util.UserError{
			Message: "File too large",
			Code:    http.StatusBadRequest,
		}
	}

	subdomain := r.FormValue("subdomain")
	if subdomain == "" {
		log.Printf("error reading subdomain: %v\n", err)
		return "", util.UserError{
			Message: "Subdomain is required",
			Code:    http.StatusBadRequest,
		}
	}

	file, header, err := r.FormFile("folder")
	if err != nil {
		log.Printf("error reading file: %v\n", err)
		return "", util.UserError{
			Message: "Invalid file",
			Code:    http.StatusBadRequest,
		}
	}
	defer file.Close()

	if !isValidFileType(header.Filename) {
		log.Printf("invalid file extension for `%s'\n", header.Filename)
		return "", util.UserError{
			Message: "Must upload a .zip file",
			Code:    http.StatusBadRequest,
		}
	}

	log.Printf("user: %d", session.UserID)
	log.Printf("Subdomain: %s\n", subdomain)
	log.Printf("Uploaded file: %s\n", header.Filename)
	log.Printf("File size: %d bytes\n", header.Size)
	log.Printf("File header MIME type: %s\n", header.Header.Get("Content-Type"))

	/* create to tmp file */
	tmpFile, err := os.CreateTemp("", "uploaded-*.zip")
	if err != nil {
		return "", fmt.Errorf("error creating tmp file: %w", err)
	}
	defer tmpFile.Close()

	/* copy uploaded file to tmpFile */
	if _, err = io.Copy(tmpFile, file); err != nil {
		return "", fmt.Errorf("error copying upload to temp file: %w", err)
	}

	src := tmpFile.Name()
	dst := buildFolderPath(session.UserID, subdomain)

	log.Printf("src: %s\n", tmpFile.Name())
	log.Printf("dst: %s\n", dst)

	/* extract to disk for folders */
	if err := extractZip(src, dst); err != nil {
		return "", fmt.Errorf("error extracting .zip: %w", err)
	}

	/* create blog */
	blog, err := b.store.CreateBlog(context.TODO(), model.CreateBlogParams{
		UserID: session.UserID,
		GhRepositoryID: sql.NullInt64{
			Valid: false,
		},
		RepositoryPath: dst,
		Subdomain:      subdomain,
		FromAddress:    config.Config.Progstack.FromEmail,
		BlogType:       model.BlogTypeFolder,
	})
	if err != nil {
		return "", fmt.Errorf("error creating folder-based blog: %w", err)
	}

	/* add user as first subscriber */
	if err = b.store.CreateSubscriberTx(context.TODO(), model.CreateSubscriberTxParams{
		BlogID: blog.ID,
		Email:  session.Email,
	}); err != nil {
		return "", fmt.Errorf("error adding email `%d' for user `%s' as subscriber to blog `%d': %w", session.Email, session.UserID, blog.ID, err)
	}

	/* take blog live  */
	_, err = setBlogToLive(blog, b.store)
	if err != nil {
		return "", fmt.Errorf("error setting blog to live: %w", err)
	}
	return buildDomainUrl(blog.Subdomain), nil
}

func isValidFileType(filename string) bool {
	allowedExtensions := map[string]bool{
		".zip": true,
	}
	ext := filepath.Ext(filename)
	return allowedExtensions[ext]
}

func buildFolderPath(userID int32, subdomain string) string {
	userIDString := strconv.FormatInt(int64(userID), 10)
	return filepath.Join(
		config.Config.Progstack.FoldersPath,
		userIDString,
		uuid.New().String(),
	)
}

func extractZip(zipPath, dest string) error {
	/* open zip file */
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip file: %w", err)
	}
	defer r.Close()

	/* loop over files in zip archive */
	for _, f := range r.File {
		/* create the destination path by removing the top-level * directory */
		destPath := filepath.Join(dest, f.Name)

		/* remove the top-level directory if it exists */
		if strings.Contains(destPath, "/") {
			parts := strings.SplitN(f.Name, "/", 2) /* split only on the first "/" */
			if len(parts) > 1 {
				destPath = filepath.Join(dest, parts[1]) /* use the second part onwards */
			}
		}

		/* ensure the directory exists */
		if err := os.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		/* check if the current file is actually a directory */
		if f.FileInfo().IsDir() {
			/* skip creating a file for directories */
			continue
		}

		/* open the file inside the zip archive */
		srcFile, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in zip: %w", err)
		}
		defer srcFile.Close()

		/* create the destination file */
		dstFile, err := os.Create(destPath)
		if err != nil {
			return fmt.Errorf("failed to create destination file: %w", err)
		}
		defer dstFile.Close()

		/* copy the content */
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return fmt.Errorf("failed to copy file contents: %w", err)
		}
	}
	return nil
}

/* prevents directory traversal attach */
func isValidPath(fullPath, basePath string) bool {
	relPath, err := filepath.Rel(basePath, fullPath)
	if err != nil || relPath == ".." || filepath.IsAbs(relPath) {
		return false
	}
	return true
}
