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
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
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
	Theme        string `json:"theme"`
	TestBranch   string `json:"test_branch"`
	LiveBranch   string `json:"live_branch"`
	Flow         string `json:"flow"`
}

func (b *BlogService) createRepositoryBlog(w http.ResponseWriter, r *http.Request) (string, error) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
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
		return "", fmt.Errorf("could not get repository for ghRepoId `%d': %w", intRepoID, err)
	}

	theme, err := validateTheme(req.Theme)
	if err != nil {
		return "", err
	}

	blog, err := b.store.CreateBlog(context.TODO(), model.CreateBlogParams{
		UserID: sesh.GetUserID(),
		GhRepositoryID: sql.NullInt64{
			Valid: true,
			Int64: intRepoID,
		},
		GhUrl: sql.NullString{
			Valid:  true,
			String: buildRepositoryUrl(repo.FullName),
		},
		RepositoryPath: buildRepositoryPath(repo.FullName),
		Theme:          theme,
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
		Email:  sesh.GetEmail(),
	}); err != nil {
		return "", fmt.Errorf("error creating first subscriber: %w", err)
	}

	if !blog.GhRepositoryID.Valid {
		return "", fmt.Errorf("invalid blog repositoryID")
	}

	/* pull latest changes for live branch */
	if err := UpdateRepositoryOnDisk(b.client, b.store, blog.GhRepositoryID.Int64); err != nil {
		return "", fmt.Errorf("error pulling latest changes on live branch: %w", err)
	}

	/* take blog live  */
	_, err = SetBlogToLive(blog, b.store)
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

func validateTheme(theme string) (model.BlogTheme, error) {
	switch theme {
	case "lit":
		return model.BlogThemeLit, nil
	case "latex":
		return model.BlogThemeLatex, nil
	default:
		return "", fmt.Errorf("`%s' is not a supported theme", theme)
	}
}

func buildRepositoryPath(repoFullName string) string {
	return filepath.Join(
		config.Config.Progstack.RepositoriesPath,
		repoFullName,
	)
}

func UpdateRepositoryOnDisk(c *httpclient.Client, s *model.Store, ghRepositoryId int64) error {
	log.Printf("updating repository `%d' on disk...", ghRepositoryId)

	/* get repository */
	repo, err := s.GetRepositoryByGhRepositoryID(context.TODO(), ghRepositoryId)
	if err != nil {
		return err
	}

	/* get blog */
	blog, err := s.GetBlogByGhRepositoryID(context.TODO(), sql.NullInt64{
		Valid: true,
		Int64: ghRepositoryId,
	})
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("error getting blog for repository event: %w", err)
		}
		/* XXX: can happen if user pushes to repo after installing
		* application without having created an associated blog*/
		log.Printf("no associated blog with repositoryID `%d'\n", ghRepositoryId)
		return nil
	}

	accessToken, err := auth.GetInstallationAccessToken(
		c,
		config.Config.Github.AppID,
		repo.InstallationID,
		config.Config.Github.PrivateKeyPath,
	)
	if err != nil {
		return fmt.Errorf("error getting installation access token: %w", err)
	}

	if !blog.LiveBranch.Valid {
		return fmt.Errorf("no live branch configured for blog `%d'", blog.ID)
	}

	/* download live branch tarball */
	tmpFile, err := downloadRepoTarball(c, repo.FullName, blog.LiveBranch.String, accessToken)
	if err != nil {
		return fmt.Errorf("error downloading tarball for at url: %s: %w", repo.FullName, err)
	}

	/* extract tarball to destination should store under user */
	tmpDst := buildRepositoryPath(repo.FullName)
	if err = extractTarball(tmpFile, tmpDst); err != nil {
		return fmt.Errorf("error extracting tarball to destination for `%s': %w", repo.FullName, err)
	}
	return nil
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
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		log.Println("user not found")
		return "", util.UserError{
			Message: "",
			Code:    http.StatusNotFound,
		}
	}

	req, err := parseCreateFolderBlogRequest(r)
	if err != nil {
		return "", err
	}

	userIDString := strconv.FormatInt(int64(sesh.GetUserID()), 10)
	dst := buildFolderPath(userIDString)

	log.Printf("src: %s\n", req.src)
	log.Printf("dst: %s\n", dst)

	/* extract to disk for folders */
	if err := extractZip(req.src, dst); err != nil {
		return "", fmt.Errorf("error extracting .zip: %w", err)
	}

	/* create blog */
	blog, err := b.store.CreateBlog(context.TODO(), model.CreateBlogParams{
		UserID: sesh.GetUserID(),
		GhRepositoryID: sql.NullInt64{
			Valid: false,
		},
		RepositoryPath: dst,
		Subdomain:      req.subdomain,
		FromAddress:    config.Config.Progstack.FromEmail,
		Theme:          req.theme,
		BlogType:       model.BlogTypeFolder,
	})
	if err != nil {
		return "", fmt.Errorf("error creating folder-based blog: %w", err)
	}

	/* add user as first subscriber */
	if err = b.store.CreateSubscriberTx(context.TODO(), model.CreateSubscriberTxParams{
		BlogID: blog.ID,
		Email:  sesh.GetEmail(),
	}); err != nil {
		return "", fmt.Errorf("error adding email `%s' for user `%d' as subscriber to blog `%d': %w", sesh.GetEmail(), sesh.GetUserID(), blog.ID, err)
	}

	/* take blog live  */
	_, err = SetBlogToLive(blog, b.store)
	if err != nil {
		return "", fmt.Errorf("error setting blog to live: %w", err)
	}
	return buildDomainUrl(blog.Subdomain), nil
}

type createFolderBlogRequest struct {
	subdomain string
	src       string
	theme     model.BlogTheme
}

func parseCreateFolderBlogRequest(r *http.Request) (createFolderBlogRequest, error) {
	/* XXX: Add subscription based file size limits */
	err := r.ParseMultipartForm(maxFileSize) /* 10MB limit */
	if err != nil {
		log.Printf("file too large: %v\n", err)
		return createFolderBlogRequest{}, util.UserError{
			Message: "File too large",
			Code:    http.StatusBadRequest,
		}
	}

	subdomain := r.FormValue("subdomain")
	if subdomain == "" {
		log.Println("error reading subdomain")
		return createFolderBlogRequest{}, util.UserError{
			Message: "Subdomain is required",
			Code:    http.StatusBadRequest,
		}
	}

	file, header, err := r.FormFile("folder")
	if err != nil {
		log.Printf("error reading file: %v\n", err)
		return createFolderBlogRequest{}, util.UserError{
			Message: "Invalid file",
			Code:    http.StatusBadRequest,
		}
	}
	defer file.Close()

	if !isValidFileType(header.Filename) {
		log.Printf("invalid file extension for `%s'\n", header.Filename)
		return createFolderBlogRequest{}, util.UserError{
			Message: "Must upload a .zip file",
			Code:    http.StatusBadRequest,
		}
	}

	/* create to tmp file */
	tmpFile, err := os.CreateTemp("", "uploaded-*.zip")
	if err != nil {
		return createFolderBlogRequest{}, fmt.Errorf("error creating tmp file: %w", err)
	}
	defer tmpFile.Close()

	/* copy uploaded file to tmpFile */
	if _, err = io.Copy(tmpFile, file); err != nil {
		return createFolderBlogRequest{}, fmt.Errorf("error copying upload to temp file: %w", err)
	}

	/* theme */
	theme, err := validateTheme(r.FormValue("theme"))
	if err != nil {
		log.Printf("error reading theme")
		return createFolderBlogRequest{}, util.UserError{
			Message: "Invalid theme",
			Code:    http.StatusBadRequest,
		}
	}

	return createFolderBlogRequest{
		subdomain: subdomain,
		src:       tmpFile.Name(),
		theme:     theme,
	}, nil
}

func isValidFileType(filename string) bool {
	allowedExtensions := map[string]bool{
		".zip": true,
	}
	ext := filepath.Ext(filename)
	return allowedExtensions[ext]
}

func buildFolderPath(userID string) string {
	return filepath.Join(
		config.Config.Progstack.FoldersPath,
		userID,
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
