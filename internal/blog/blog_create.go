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
	"github.com/xr0-org/progstack-ssg/pkg/ssg"
	"github.com/xr0-org/progstack/internal/authn"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/dns"
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

	repopath := buildRepositoryPath(repo.FullName)

	if err := UpdateRepositoryOnDisk(
		b.client, b.store, intRepoID, req.LiveBranch,
		logger,
	); err != nil {
		return "", fmt.Errorf("error pulling latest changes on live branch: %w", err)
	}

	h, err := ssg.GetSiteHash(repopath)
	if err != nil {
		return "", fmt.Errorf("cannot get hash: %w", err)
	}

	sub, err := dns.ParseSubdomain(req.Subdomain)
	if err != nil {
		return "", fmt.Errorf("subdomain: %w", err)
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
		RepositoryPath: repopath,
		Theme:          theme,
		Subdomain:      sub,
		TestBranch: sql.NullString{
			Valid:  true,
			String: req.TestBranch,
		},
		LiveBranch: sql.NullString{
			Valid:  true,
			String: req.LiveBranch,
		},
		LiveHash:  h,
		BlogType:  model.BlogTypeRepository,
		EmailMode: model.EmailModePlaintext,
		FromAddress: fmt.Sprintf(
			"%s@%s",
			sub, config.Config.Progstack.EmailDomain,
		),
	})
	if err != nil {
		return "", fmt.Errorf("could not create blog: %w", err)
	}

	// add owner as subscriber
	if _, err = b.store.CreateSubscriber(
		context.TODO(),
		model.CreateSubscriberParams{
			BlogID: blog.ID,
			Email:  sesh.GetEmail(),
		},
	); err != nil {
		return "", fmt.Errorf("error subscribing owner: %w", err)
	}

	if !blog.GhRepositoryID.Valid {
		return "", fmt.Errorf("invalid blog repositoryID")
	}

	if _, err := setBlogToLive(&blog, b.store, logger); err != nil {
		return "", fmt.Errorf("error setting blog to live: %w", err)
	}
	return buildUrl(blog.Subdomain.String()), nil
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
	c *httpclient.Client, s *model.Store, ghRepoId int64, branch string,
	logger *log.Logger,
) error {
	logger.Printf("updating repository `%d' on disk...\n", ghRepoId)

	/* get repository */
	repo, err := s.GetRepositoryByGhRepositoryID(context.TODO(), ghRepoId)
	if err != nil {
		return err
	}

	accessToken, err := authn.GetInstallationAccessToken(
		c,
		config.Config.Github.AppID,
		repo.InstallationID,
		config.Config.Github.PrivateKeyPath,
	)
	if err != nil {
		return fmt.Errorf("access token error: %w", err)
	}

	if err := cloneRepo(
		buildRepositoryPath(repo.FullName),
		repo.Url,
		branch,
		accessToken,
	); err != nil {
		return fmt.Errorf("clone error: %w", err)
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

	h, err := ssg.GetSiteHash(dst)
	if err != nil {
		return "", fmt.Errorf("cannot get hash: %w", err)
	}

	sub, err := dns.ParseSubdomain(req.subdomain)
	if err != nil {
		return "", fmt.Errorf("subdomain: %w", err)
	}

	blog, err := b.store.CreateBlog(context.TODO(), model.CreateBlogParams{
		UserID: sesh.GetUserID(),
		GhRepositoryID: sql.NullInt64{
			Valid: false,
		},
		RepositoryPath: dst,
		Subdomain:      sub,
		Theme:          req.theme,
		BlogType:       model.BlogTypeFolder,
		LiveHash:       h,
		FromAddress: fmt.Sprintf(
			"%s@%s",
			sub, config.Config.Progstack.EmailDomain,
		),
	})
	if err != nil {
		return "", fmt.Errorf("error creating folder-based blog: %w", err)
	}

	// subscribe owner
	if _, err := b.store.CreateSubscriber(
		context.TODO(),
		model.CreateSubscriberParams{
			BlogID: blog.ID,
			Email:  sesh.GetEmail(),
		},
	); err != nil {
		return "", fmt.Errorf("error subscribing owner: %w", err)
	}
	if _, err := setBlogToLive(&blog, b.store, logger); err != nil {
		return "", fmt.Errorf("error setting blog to live: %w", err)
	}
	return buildUrl(blog.Subdomain.String()), nil
}

type createFolderBlogRequest struct {
	subdomain string
	src       string
	theme     model.BlogTheme
}

func parseCreateFolderBlogRequest(r *http.Request) (createFolderBlogRequest, error) {
	logger := logging.Logger(r)

	/* XXX: Add subscription based file size limits */
	if err := r.ParseMultipartForm(maxFileSize); err != nil {
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
