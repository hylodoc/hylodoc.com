package blog

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/google/uuid"
	"github.com/xr0-org/progstack-ssg/pkg/ssg"
	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/authn"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/dns"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
)

type CreateBlogResponse struct {
	Url     string `json:"url"`
	Message string `json:"message"`
}

func (b *BlogService) CreateRepositoryBlog(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("CreateRepositoryBlog handler...")

	r.MixpanelTrack("CreateRepositoryBlog")

	var req struct {
		Subdomain    string `json:"subdomain"`
		RepositoryID string `json:"repository_id"`
		Theme        string `json:"theme"`
		TestBranch   string `json:"test_branch"`
		LiveBranch   string `json:"live_branch"`
		Flow         string `json:"flow"`
	}
	body, err := r.ReadBody()
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, createCustomError(
			"error decoding request body",
			http.StatusBadRequest,
		)
	}

	intRepoID, err := strconv.ParseInt(req.RepositoryID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf(
			"convert repositoryID `%s' to int64: %w",
			req.RepositoryID, err,
		)
	}

	theme, err := validateTheme(req.Theme)
	if err != nil {
		return nil, fmt.Errorf("validate theme: %w", err)
	}

	sub, err := dns.ParseSubdomain(req.Subdomain)
	if err != nil {
		return nil, fmt.Errorf("parse subdomain: %w", err)
	}

	userid, err := sesh.GetUserID()
	if err != nil {
		return nil, fmt.Errorf("get user id: %w", err)
	}
	blog, err := b.store.CreateBlog(context.TODO(), model.CreateBlogParams{
		UserID: userid,
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
		return nil, fmt.Errorf("create blog: %w", err)
	}

	if err := UpdateRepositoryOnDisk(
		b.client, b.store, &blog, sesh,
	); err != nil {
		return nil, fmt.Errorf("update repo on disk: %w", err)
	}

	// add owner as subscriber
	if _, err = b.store.CreateSubscriber(
		context.TODO(),
		model.CreateSubscriberParams{
			BlogID: blog.ID,
			Email:  sesh.GetEmail(),
		},
	); err != nil {
		return nil, fmt.Errorf("subscribe owner: %w", err)
	}

	if !blog.GhRepositoryID.Valid {
		return nil, fmt.Errorf("invalid blog repositoryID")
	}

	if _, err := setBlogToLive(&blog, b.store, sesh); err != nil {
		return nil, fmt.Errorf("set blog to live: %w", err)
	}
	return response.NewJson(
		CreateBlogResponse{
			Url:     buildUrl(blog.Subdomain.String()),
			Message: "Successfully created repository-based blog!",
		},
	)
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
	sesh *session.Session,
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

func (b *BlogService) CreateFolderBlog(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("CreateFolderBlog handler...")

	subdomain, err := r.GetFormValue("subdomain")
	if err != nil {
		return nil, createCustomError(
			"Invalid subdomain", http.StatusBadRequest,
		)
	}
	if subdomain == "" {
		return nil, createCustomError(
			"Subdomain is required", http.StatusBadRequest,
		)
	}
	rawtheme, err := r.GetFormValue("theme")
	if err != nil {
		return nil, fmt.Errorf("get theme: %w", err)
	}
	theme, err := validateTheme(rawtheme)
	if err != nil {
		return nil, createCustomError(
			"Invalid theme",
			http.StatusBadRequest,
		)
	}

	folderpath, err := getUploadedFolderPath(r)
	if err != nil {
		return nil, fmt.Errorf("get uploaded folder path: %w", err)
	}

	userid, err := sesh.GetUserID()
	if err != nil {
		return nil, fmt.Errorf("get user id: %w", err)
	}
	dst := filepath.Join(
		config.Config.Progstack.FoldersPath,
		strconv.FormatInt(int64(userid), 10),
		uuid.New().String(),
	)

	/* extract to disk for folders */
	if err := extractZip(folderpath, dst); err != nil {
		return nil, fmt.Errorf("extract .zip: %w", err)
	}

	h, err := ssg.GetSiteHash(dst)
	if err != nil {
		return nil, fmt.Errorf("get hash: %w", err)
	}

	sub, err := dns.ParseSubdomain(subdomain)
	if err != nil {
		return nil, fmt.Errorf("parse subdomain: %w", err)
	}

	blog, err := b.store.CreateBlog(context.TODO(), model.CreateBlogParams{
		UserID: userid,
		GhRepositoryID: sql.NullInt64{
			Valid: false,
		},
		FolderPath: sql.NullString{dst, true},
		Subdomain:  sub,
		Theme:      theme,
		BlogType:   model.BlogTypeFolder,
		LiveHash:   sql.NullString{h, true},
		EmailMode:  model.EmailModeHtml,
		FromAddress: fmt.Sprintf(
			"%s@%s",
			sub, config.Config.Progstack.EmailDomain,
		),
	})
	if err != nil {
		return nil, fmt.Errorf("error creating folder-based blog: %w", err)
	}

	// subscribe owner
	if _, err := b.store.CreateSubscriber(
		context.TODO(),
		model.CreateSubscriberParams{
			BlogID: blog.ID,
			Email:  sesh.GetEmail(),
		},
	); err != nil {
		return nil, fmt.Errorf("error subscribing owner: %w", err)
	}
	if _, err := setBlogToLive(&blog, b.store, sesh); err != nil {
		return nil, fmt.Errorf("error setting blog to live: %w", err)
	}
	return response.NewJson(
		CreateBlogResponse{
			Url:     buildUrl(blog.Subdomain.String()),
			Message: "Successfully created folder-based blog.",
		},
	)
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
