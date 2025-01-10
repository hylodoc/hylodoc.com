package blog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/xr0-org/progstack-ssg/pkg/ssg"
	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/authn"
	"github.com/xr0-org/progstack/internal/blog/internal/errorset"
	"github.com/xr0-org/progstack/internal/blog/internal/valueset"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/dns"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

func (b *BlogService) CreateRepositoryBlog(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("CreateRepositoryBlog handler...")

	r.MixpanelTrack("CreateRepositoryBlog")

	userid, err := sesh.GetUserID()
	if err != nil {
		return nil, fmt.Errorf("get user id: %w", err)
	}
	if err := b.awaitupdate(userid); err != nil {
		return nil, fmt.Errorf("await update: %w", err)
	}
	return b.createBlogPage(r, valueset.NewEmpty(), errorset.NewEmpty())
}

func (b *BlogService) createBlogPage(
	r request.Request, values valueset.Set, errors errorset.Set,
) (response.Response, error) {
	sesh := r.Session()
	userid, err := sesh.GetUserID()
	if err != nil {
		return nil, fmt.Errorf("get user id: %w", err)
	}

	repos, err := b.store.ListOrderedRepositoriesByUserID(
		context.TODO(), userid,
	)
	if err != nil {
		return nil, fmt.Errorf("get repositories: %w", err)
	}
	hasinstallation, err := b.store.InstallationExistsForUserID(
		context.TODO(), userid,
	)
	if err != nil {
		return nil, fmt.Errorf("has installation: %w", err)
	}
	islinked, err := userIsLinked(userid, b.store)
	if err != nil {
		return nil, fmt.Errorf("user is linked: %w", err)
	}

	return response.NewTemplate(
		[]string{"blog_repository_flow.html"},
		util.PageInfo{
			Data: struct {
				Title           string
				UserInfo        *session.UserInfo
				HasInstallation bool
				IsLinked        bool
				RootDomain      string
				Repositories    []repository
				Themes          []string
				Values          valueset.Set
				Errors          errorset.Set
			}{
				Title:           "Create new blog",
				UserInfo:        session.ConvertSessionToUserInfo(sesh),
				HasInstallation: hasinstallation,
				IsLinked:        islinked,
				RootDomain:      config.Config.Progstack.RootDomain,
				Repositories:    buildRepositories(repos),
				Themes:          BuildThemes(config.Config.ProgstackSsg.Themes),
				Values:          values,
				Errors:          errors,
			},
		},
	), nil
}

func (b *BlogService) awaitupdate(userID int32) error {
	/* TODO: get from config */
	var (
		timeout = 5 * time.Second
		step    = 100 * time.Millisecond
	)
	now := time.Now
	for until := now().Add(timeout); now().Before(until); time.Sleep(step) {
		awaiting, err := b.store.IsAwaitingGithubUpdate(
			context.TODO(), userID,
		)
		if err != nil {
			return fmt.Errorf("check if awaiting: %w", err)
		}
		if !awaiting {
			return nil
		}
	}
	if err := b.store.UpdateAwaitingGithubUpdate(
		context.TODO(),
		model.UpdateAwaitingGithubUpdateParams{
			ID:               userID,
			GhAwaitingUpdate: false,
		},
	); err != nil {
		return fmt.Errorf("update github update: %w", err)
	}
	return fmt.Errorf("timeout")
}

type repository struct {
	Value int64
	Name  string
}

func buildRepositories(dbrepos []model.Repository) []repository {
	res := make([]repository, len(dbrepos))
	for i, dbrepo := range dbrepos {
		res[i] = repository{dbrepo.RepositoryID, dbrepo.FullName}
	}
	return res
}

func userIsLinked(userid int32, s *model.Store) (bool, error) {
	if _, err := s.GetGithubAccountByUserID(
		context.TODO(), userid,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("get github account: %w", err)
	}
	return true, nil
}

type CreateBlogResponse struct {
	Url     string `json:"url"`
	Message string `json:"message"`
}

func (b *BlogService) SubmitRepositoryBlog(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("SubmitRepositoryBlog handler...")

	r.MixpanelTrack("SubmitRepositoryBlog")

	rawRepoID, err := r.GetPostFormValue("repositoriesDropdown")
	if err != nil {
		return nil, fmt.Errorf("get rawRepoID: %w", err)
	}
	intRepoID, err := strconv.ParseInt(rawRepoID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf(
			"convert repositoryID `%s' to int64: %w",
			rawRepoID, err,
		)
	}

	rawSubdomain, err := r.GetPostFormValue("subdomainInput")
	if err != nil {
		return nil, fmt.Errorf("get rawSubdomain: %w", err)
	}
	sub, err := dns.ParseSubdomain(rawSubdomain)
	if err != nil {
		return nil, fmt.Errorf("parse subdomain: %w", err)
	}

	rawTheme, err := r.GetPostFormValue("themesDropdown")
	if err != nil {
		return nil, fmt.Errorf("get rawTheme: %w", err)
	}
	theme, err := getTheme(rawTheme)
	if err != nil {
		return nil, fmt.Errorf("get theme: %w", err)
	}

	branch, err := r.GetPostFormValue("liveBranchInput")
	if err != nil {
		return nil, fmt.Errorf("get branch: %w", err)
	}

	if err := b.store.ExecTx(
		func(s *model.Store) error {
			return createBlogTx(
				intRepoID, &theme, sub, branch,
				b.client, sesh, s,
			)
		},
	); err != nil {
		if errors.Is(err, ssg.ErrTheme) {
			return b.createBlogPage(
				r,
				valueset.New(
					intRepoID,
					sub.String(),
					rawTheme,
					branch,
				),
				errorset.NewTheme("Theme is broken"),
			)
		}
		return nil, fmt.Errorf("create blog tx: %w", err)
	}
	return response.NewRedirect(
		buildUrl(sub.String()), http.StatusTemporaryRedirect,
	), nil
}

func createBlogTx(
	ghRepoID int64, theme *model.BlogTheme, sub *dns.Subdomain,
	livebranch string,
	c *httpclient.Client, sesh *session.Session, s *model.Store,
) error {
	userid, err := sesh.GetUserID()
	if err != nil {
		return fmt.Errorf("get user id: %w", err)
	}

	blog, err := s.CreateBlog(
		context.TODO(),
		model.CreateBlogParams{
			UserID:         userid,
			GhRepositoryID: ghRepoID,
			Theme:          *theme,
			Subdomain:      sub,
			LiveBranch:     livebranch,
			EmailMode:      model.EmailModeHtml,
			FromAddress: fmt.Sprintf(
				"%s@%s",
				sub, config.Config.Progstack.EmailDomain,
			),
		},
	)
	if err != nil {
		return fmt.Errorf("create blog: %w", err)
	}

	if err := UpdateRepositoryOnDisk(c, &blog, sesh, s); err != nil {
		return fmt.Errorf("update repo on disk: %w", err)
	}

	/* add owner as subscriber */
	if _, err = s.CreateSubscriber(
		context.TODO(),
		model.CreateSubscriberParams{
			BlogID: blog.ID,
			Email:  sesh.GetEmail(),
		},
	); err != nil {
		return fmt.Errorf("subscribe owner: %w", err)
	}

	if _, err := setBlogToLive(&blog, sesh, s); err != nil {
		return fmt.Errorf("set blog to live: %w", err)
	}

	return nil
}

func buildRepositoryUrl(fullName string) string {
	return fmt.Sprintf(
		"https://github.com/%s/",
		fullName,
	)
}

func getTheme(theme string) (model.BlogTheme, error) {
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
	c *httpclient.Client, blog *model.Blog,
	sesh *session.Session, s *model.Store,
) error {
	repo, err := s.GetRepositoryByGhRepositoryID(
		context.TODO(), blog.GhRepositoryID,
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
	h, err := updateAndCheckout(
		repo.Url, repo.GitdirPath, blog.LiveBranch, accessToken,
	)
	if err != nil {
		return fmt.Errorf("update and checkout: %w", err)
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

// updateAndCheckout clones the repo at the given URL into a bare git dir as
// provided, and then clones and checks out locally the branch given by its
// latest hash into config.Config.Progstack.CheckoutsPath. It returns this
// latest hash.
//
// If gitdir already exists, it is removed before the cloning.
func updateAndCheckout(repoURL, gitdir, branch, token string) (string, error) {
	if err := removeDirIfExists(gitdir); err != nil {
		return "", fmt.Errorf("remove gitdir if exists: %w", err)
	}
	refname := plumbing.NewBranchReferenceName(branch)
	repo, err := git.PlainClone(
		gitdir,
		true,
		&git.CloneOptions{
			URL: repoURL,
			Auth: &githttp.BasicAuth{
				Username: "github", Password: token,
			},
			ReferenceName: refname,
		},
	)
	if err != nil {
		return "", fmt.Errorf("bare clone: %w", err)
	}

	ref, err := repo.Reference(refname, true)
	if err != nil {
		return "", fmt.Errorf("reference: %w", err)
	}
	h := ref.Hash().String()

	checkoutdir := filepath.Join(config.Config.Progstack.CheckoutsPath, h)
	if err := removeDirIfExists(checkoutdir); err != nil {
		return "", fmt.Errorf("remove checkoutdir if exists: %w", err)
	}
	if _, err := git.PlainClone(
		checkoutdir,
		false,
		&git.CloneOptions{
			URL:           gitdir,
			ReferenceName: refname,
		},
	); err != nil {
		return "", fmt.Errorf("checkout clone: %w", err)
	}

	return h, nil
}

func removeDirIfExists(dir string) error {
	_, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stat: %w", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove: %w", err)
	}
	return nil
}
