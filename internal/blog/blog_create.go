package blog

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	"github.com/xr0-org/progstack/internal/logging"
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
		logger := logging.Logger(r)
		logger.Println("CreateRepositoryBlog handler...")

		b.mixpanel.Track("CreateRepositoryBlog", r)

		message := "Successfully created repository-based blog!"
		url, err := b.createRepositoryBlog(w, r)
		if err != nil {
			var customErr *util.CustomError
			if errors.As(err, &customErr) {
				logger.Printf("Client Error: %v\n", customErr)
				http.Error(w, customErr.Error(), http.StatusBadRequest)
				return
			} else {
				logger.Printf("Internal Server Error: %v\n", err)
				http.Error(w, "", http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(CreateBlogResponse{
			Url:     url,
			Message: message,
		}); err != nil {
			logger.Printf("Error encoding response: %v\n", err)
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
	logger := logging.Logger(r)

	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		return "", fmt.Errorf("user not found")
	}

	var req CreateRepositoryBlogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Printf("Error decoding body: %v\n", err)
		return "", util.CreateCustomError(
			"error decoding request body",
			http.StatusBadRequest,
		)
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
	if err := UpdateRepositoryOnDisk(
		b.client, b.store, blog.GhRepositoryID.Int64, logger,
	); err != nil {
		return "", fmt.Errorf("error pulling latest changes on live branch: %w", err)
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

func UpdateRepositoryOnDisk(
	c *httpclient.Client, s *model.Store, ghRepositoryId int64,
	logger *log.Logger,
) error {
	logger.Printf("updating repository `%d' on disk...", ghRepositoryId)

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
		logger.Printf("No associated blog with repositoryID `%d'\n", ghRepositoryId)
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

	if err := cloneRepo(
		buildRepositoryPath(repo.FullName),
		repo.Url,
		blog.LiveBranch.String,
		accessToken,
	); err != nil {
		return fmt.Errorf("clone error: %w", err)
	}

	/* take blog live  */
	if _, err := setBlogToLive(&blog, s, logger); err != nil {
		return fmt.Errorf("error setting blog to live: %w", err)
	}
	return nil
}

func (b *BlogService) CreateFolderBlog() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)
		logger.Println("CreateFolderBlog handler...")

		message := "Successfully created folder-based blog."
		url, err := b.createFolderBlog(w, r)
		if err != nil {
			var customErr *util.CustomError
			if errors.As(err, &customErr) {
				logger.Printf("Client Error: %v\n", customErr)
				message = customErr.Error()
			} else {
				/* internal error */
				logger.Printf("Internal Server Error: %v\n", err)
				http.Error(w, "", http.StatusInternalServerError)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		if err = json.NewEncoder(w).Encode(CreateBlogResponse{
			Url:     url,
			Message: message,
		}); err != nil {
			logger.Printf("Error encoding response: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}
}

func (b *BlogService) createFolderBlog(w http.ResponseWriter, r *http.Request) (string, error) {
	logger := logging.Logger(r)

	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		logger.Println("No auth session")
		return "", util.CreateCustomError("", http.StatusNotFound)
	}

	req, err := parseCreateFolderBlogRequest(r)
	if err != nil {
		return "", err
	}

	userIDString := strconv.FormatInt(int64(sesh.GetUserID()), 10)
	dst := buildFolderPath(userIDString)

	logger.Printf("src: %s\n", req.src)
	logger.Printf("dst: %s\n", dst)

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
		return "", fmt.Errorf(
			"error adding email `%s' for user `%d' as subscriber to blog `%d': %w",
			sesh.GetEmail(),
			sesh.GetUserID(),
			blog.ID,
			err,
		)
	}

	/* take blog live  */
	if _, err := setBlogToLive(&blog, b.store, logger); err != nil {
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
	logger := logging.Logger(r)

	/* XXX: Add subscription based file size limits */
	err := r.ParseMultipartForm(maxFileSize) /* 10MB limit */
	if err != nil {
		logger.Printf("Error File too large: %v\n", err)
		return createFolderBlogRequest{}, util.CreateCustomError(
			"File too large",
			http.StatusBadRequest,
		)
	}

	subdomain := r.FormValue("subdomain")
	if subdomain == "" {
		logger.Println("error reading subdomain")
		return createFolderBlogRequest{}, util.CreateCustomError(
			"Subdomain is required",
			http.StatusBadRequest,
		)
	}

	file, header, err := r.FormFile("folder")
	if err != nil {
		logger.Printf("Error reading file: %v\n", err)
		return createFolderBlogRequest{}, util.CreateCustomError(
			"Invalid file",
			http.StatusBadRequest,
		)
	}
	defer file.Close()

	if !isValidFileType(header.Filename) {
		logger.Printf("Invalid file extension for `%s'\n", header.Filename)
		return createFolderBlogRequest{}, util.CreateCustomError(
			"Must upload a .zip file",
			http.StatusBadRequest,
		)
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
		logger.Printf("Error reading theme")
		return createFolderBlogRequest{}, util.CreateCustomError(
			"Invalid theme",
			http.StatusBadRequest,
		)
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
