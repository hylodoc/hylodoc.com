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

	"github.com/google/uuid"
	"github.com/xr0-org/progstack-ssg/pkg/ssg"
	"github.com/xr0-org/progstack/internal/assert"
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

func (b *BlogService) createRepositoryBlog(
	w http.ResponseWriter, r *http.Request,
) (string, error) {
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

	theme, err := validateTheme(req.Theme)
	if err != nil {
		return "", err
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
		Theme:     theme,
		Subdomain: sub,
		TestBranch: sql.NullString{
			Valid:  true,
			String: req.TestBranch,
		},
		LiveBranch: sql.NullString{
			Valid:  true,
			String: req.LiveBranch,
		},
		BlogType:  model.BlogTypeRepository,
		EmailMode: model.EmailModeHtml,
		FromAddress: fmt.Sprintf(
			"%s@%s",
			sub, config.Config.Progstack.EmailDomain,
		),
	})
	if err != nil {
		return "", fmt.Errorf("could not create blog: %w", err)
	}

	if err := UpdateRepositoryOnDisk(
		b.client, b.store, &blog, logger,
	); err != nil {
		return "", fmt.Errorf("error pulling latest changes on live branch: %w", err)
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

func UpdateRepositoryOnDisk(
	c *httpclient.Client, s *model.Store, blog *model.Blog,
	logger *log.Logger,
) error {
	assert.Assert(blog.GhRepositoryID.Valid)
	repo, err := s.GetRepositoryByGhRepositoryID(
		context.TODO(), blog.GhRepositoryID.Int64,
	)
	if err != nil {
		return fmt.Errorf("get repo: %w", err)
	}
	accessToken, err := authn.GetInstallationAccessToken(
		c,
		config.Config.Github.AppID,
		repo.InstallationID,
		config.Config.Github.PrivateKeyPath,
	)
	if err != nil {
		return fmt.Errorf("access token: %w", err)
	}
	assert.Assert(blog.LiveBranch.Valid)
	if err := cloneRepo(
		repo.PathOnDisk,
		repo.Url,
		blog.LiveBranch.String,
		accessToken,
	); err != nil {
		return fmt.Errorf("clone: %w", err)
	}
	h, err := ssg.GetSiteHash(repo.PathOnDisk)
	if err != nil {
		return fmt.Errorf("get site hash: %w", err)
	}
	if err := s.UpdateBlogLiveHash(
		context.TODO(),
		model.UpdateBlogLiveHashParams{
			ID:       blog.ID,
			LiveHash: h,
		},
	); err != nil {
		return fmt.Errorf("update live hash: %w", err)
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

	dst := filepath.Join(
		config.Config.Progstack.FoldersPath,
		strconv.FormatInt(int64(sesh.GetUserID()), 10),
		uuid.New().String(),
	)

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
		FolderPath: sql.NullString{dst, true},
		Subdomain:  sub,
		Theme:      req.theme,
		BlogType:   model.BlogTypeFolder,
		LiveHash:   sql.NullString{h, true},
		EmailMode:  model.EmailModeHtml,
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

func extractZip(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("read zip: %w", err)
	}
	defer r.Close()
	for _, f := range r.File {
		destPath := filepath.Join(dest, f.Name)

		/* ensure directory exists */
		if err := os.MkdirAll(
			filepath.Dir(destPath), os.ModePerm,
		); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}

		if f.FileInfo().IsDir() {
			/* skip creating file for directories */
			continue
		}

		if err := copyzippedfile(f, destPath); err != nil {
			return fmt.Errorf("copy: %w", err)
		}
	}
	return nil
}

func copyzippedfile(f *zip.File, dstPath string) error {
	src, err := f.Open()
	if err != nil {
		return fmt.Errorf("src: %w", err)
	}
	defer src.Close()
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("dst: %w", err)
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return err
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
