package installation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/authn"
	"github.com/xr0-org/progstack/internal/blog"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

const (
	GhInstallUrlTemplate = "https://github.com/apps/%s/installations/new"

	ghInstallationRepositoriesUrl    = "https://api.github.com/installation/repositories"
	ghAccessTokenUrlTemplate         = "https://api.github.com/app/installations/%d/access_tokens"
	ghRepositoriesTarballUrlTemplate = "https://api.github.com/repos/%s/tarball/%s"
)

type InstallationService struct {
	client *httpclient.Client
	store  *model.Store
}

func NewInstallationService(
	c *httpclient.Client, s *model.Store,
) *InstallationService {
	return &InstallationService{
		client: c,
		store:  s,
	}
}

func (i *InstallationService) InstallationCallback(
	r request.Request,
) (response.Response, error) {
	sesh := r.Session()
	sesh.Println("InstallationCallback handler...")
	if err := i.installationcallback(r); err != nil {
		return nil, err
	}
	return response.NewEmpty(), nil
}

func (i *InstallationService) installationcallback(
	r request.Request,
) error {
	sesh := r.Session()

	/* validate authenticity using Github webhook secret */
	if err := validateSignature(
		r, config.Config.Github.WebhookSecret,
	); err != nil {
		return fmt.Errorf("validate github signature: %w", err)
	}

	body, err := r.ReadBody()
	if err != nil {
		return fmt.Errorf("request body: %w", err)
	}
	switch eventType := r.GetHeader("X-GitHub-Event"); eventType {
	case "installation":
		return handleInstallation(i.client, i.store, body, sesh)
	case "installation_repositories":
		return handleInstallationRepositories(
			i.client, i.store, body, sesh,
		)
	case "push":
		return handlePush(i.client, i.store, body, sesh)
	default:
		sesh.Printf(
			"unhandled event type %q: %s\n",
			eventType, string(body),
		)
		return nil
	}
}

func handleInstallation(
	c *httpclient.Client, s *model.Store, body []byte, sesh *session.Session,
) error {
	sesh.Println("handling installation event...")

	var event InstallationEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return err
	}
	str, _ := eventToJSON(event)
	sesh.Printf("installation event: %s\n", str)

	if err := handleInstallationAction(c, s, event, sesh); err != nil {
		return fmt.Errorf("error handling installation action: %w", err)
	}

	account, err := s.GetGithubAccountByGhUserID(context.TODO(), event.Sender.ID)
	if err != nil {
		return fmt.Errorf(
			"error getting user with ghUserID `%d': %w",
			event.Sender.ID, err,
		)
	}
	if err := s.UpdateAwaitingGithubUpdate(
		context.TODO(),
		model.UpdateAwaitingGithubUpdateParams{
			ID:               account.UserID,
			GhAwaitingUpdate: false,
		},
	); err != nil {
		return fmt.Errorf("error updating awaitingGithubUpdate: %w", err)
	}

	return nil
}

func handleInstallationAction(
	c *httpclient.Client, s *model.Store, event InstallationEvent,
	sesh *session.Session,
) error {
	account, err := s.GetGithubAccountByGhUserID(context.TODO(), event.Sender.ID)
	if err != nil {
		return fmt.Errorf(
			"error getting user with ghUserID `%d': %w",
			event.Sender.ID, err,
		)
	}
	switch event.Action {
	case "created":
		return handleInstallationCreated(
			c, s, event.Installation.ID, account.UserID, account.GhEmail,
			sesh,
		)
	case "deleted":
		return handleInstallationDeleted(
			c, s, event.Installation.ID, account.UserID, sesh,
		)
	default:
		sesh.Printf("unhandled event action: %s\n", event.Action)
		return nil
	}
}

func handleInstallationCreated(
	c *httpclient.Client, s *model.Store, ghInstallationID int64,
	userID int32, ghEmail string, sesh *session.Session,
) error {
	sesh.Println("handling installation created event...")
	/* get access token */
	accessToken, err := authn.GetInstallationAccessToken(
		c,
		config.Config.Github.AppID,
		ghInstallationID,
		config.Config.Github.PrivateKeyPath,
	)
	if err != nil {
		return fmt.Errorf("error getting installation access token: %w", err)
	}

	/* get repositories from Github */
	repos, err := getReposDetails(c, accessToken, sesh)
	if err != nil {
		return fmt.Errorf("error getting repositories: %w", err)
	}
	str, _ := eventToJSON(repos)
	sesh.Printf("repos: %s\n", str)

	/* write installation and repos to db Tx */
	createInstallationTxParams := buildCreateInstallationTxParams(ghInstallationID, userID, ghEmail, repos)
	if err = s.CreateInstallationTx(context.TODO(), createInstallationTxParams); err != nil {
		/* XXX: wipe relavant repos from disk */
		return fmt.Errorf("error executing db transaction: %w", err)
	}

	return nil
}

func buildCreateInstallationTxParams(installationID int64, userID int32, ghEmail string, repos []Repository) model.InstallationTxParams {
	var iTxParams model.InstallationTxParams
	iTxParams.InstallationID = installationID
	iTxParams.UserID = userID
	iTxParams.Email = ghEmail
	var repositoryTxParams []model.RepositoryTxParams
	for _, repo := range repos {
		repositoryTxParams = append(repositoryTxParams, model.RepositoryTxParams{
			RepositoryID: repo.ID,
			Name:         repo.Name,
			FullName:     repo.FullName,
		})
	}
	iTxParams.RepositoriesTxParams = repositoryTxParams
	iTxParams.RepositoriesPath = config.Config.Progstack.RepositoriesPath
	return iTxParams
}

type InstallationRepositoriesResponse struct {
	TotalCount   int          `json:"total_count"`
	Repositories []Repository `json:"repositories"` /* XXX: reusing from events */
}

func getReposDetails(
	c *httpclient.Client, accessToken string, sesh *session.Session,
) ([]Repository, error) {
	sesh.Println("getting repositories details...")
	req, err := util.NewRequestBuilder("GET", ghInstallationRepositoriesUrl).
		WithHeader("Authorization", fmt.Sprintf("Bearer %s", accessToken)).
		WithHeader("Accept", "application/vnd.github+json").
		WithHeader("X-GitHub-Api-Version", "2022-11-28").
		Build()
	if err != nil {
		return []Repository{}, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return []Repository{}, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		sesh.Printf("error unpacking body: %v\n", err)
		return []Repository{}, err
	}

	var reposResponse InstallationRepositoriesResponse
	if err := json.Unmarshal(body, &reposResponse); err != nil {
		return []Repository{}, err
	}
	return reposResponse.Repositories, nil
}

func handleInstallationDeleted(
	c *httpclient.Client, s *model.Store, ghInstallationID int64,
	userID int32, sesh *session.Session,
) error {
	sesh.Println("handling installation deleted event...")

	/* fetch repos associated with installation */
	sesh.Printf("deleting installation %d for user %d...", ghInstallationID, userID)
	repos, err := s.ListBlogsForInstallationByGhInstallationID(context.TODO(), ghInstallationID)
	if err != nil {
		return fmt.Errorf("error getting repositories for installation %d: %w", ghInstallationID, err)
	}
	/* delete the repos on disk */
	sesh.Println("deleting repos from disk...")
	for _, repo := range repos {
		path := fmt.Sprintf("%s/%d/%s", config.Config.Progstack.RepositoriesPath, userID, repo.FullName)
		sesh.Printf("deleting repo at `%s' from disk...\n", path)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("error deleting repo `%d' from disk: %w", repo.ID, err)
		}
	}
	/* delete generated websites */
	sesh.Println("deleting generated websites from disk...")
	for _, repo := range repos {
		path := fmt.Sprintf("%s/%s", config.Config.Progstack.WebsitesPath, repo.Subdomain)
		sesh.Printf("deleting website at `%s' from disk...\n", path)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("error deleting website `%s' from disk: %w", repo.Subdomain, err)
		}
	}
	/* cascade delete the installation and associated repos */
	sesh.Printf("deleting installation with ghInstallationID `%d'...\n", ghInstallationID)
	if err = s.DeleteInstallationWithGithubInstallationID(context.TODO(), ghInstallationID); err != nil {
		return fmt.Errorf("error deleting installation: %w", err)
	}
	return nil
}

func handleInstallationRepositories(
	c *httpclient.Client, s *model.Store, body []byte, sesh *session.Session,
) error {
	sesh.Println("handling installation repositories event...")

	var event InstallationRepositoriesEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("error unmarshaling InstallationRepositoriesEvent: %w", err)
	}
	str, _ := eventToJSON(event)
	sesh.Printf("installationRepositoriesEvent: %s\n", str)

	/* check that installation exists */
	_, err := s.GetInstallationByGithubInstallationID(
		context.TODO(), event.Installation.ID,
	)
	if err != nil {
		return fmt.Errorf(
			"error getting installation with ghInstallationID: %w",
			err,
		)
	}

	if err := handleInstallationRepositoriesAction(c, s, event, sesh); err != nil {
		return fmt.Errorf(
			"error handling installation repositories action: %w",
			err,
		)
	}

	user, err := s.GetUserByGhUserID(context.TODO(), event.Sender.ID)
	if err != nil {
		return fmt.Errorf(
			"error getting user with ghUserID `%d': %w",
			event.Sender.ID, err,
		)
	}
	if err := s.UpdateAwaitingGithubUpdate(
		context.TODO(),
		model.UpdateAwaitingGithubUpdateParams{
			ID:               user.ID,
			GhAwaitingUpdate: false,
		},
	); err != nil {
		return fmt.Errorf("error updating awaitingGithubUpdate: %w", err)
	}

	return nil
}

func handleInstallationRepositoriesAction(
	c *httpclient.Client, s *model.Store, event InstallationRepositoriesEvent,
	sesh *session.Session,
) error {
	account, err := s.GetGithubAccountByGhUserID(context.TODO(), event.Sender.ID)
	if err != nil {
		return fmt.Errorf(
			"error getting user with ghUserID `%d': %w",
			event.Sender.ID, err,
		)
	}
	switch event.Action {
	case "added":
		return handleInstallationRepositoriesAdded(
			c,
			s,
			event.Installation.ID,
			event.RepositoriesAdded,
			account.UserID,
			account.GhEmail,
			sesh,
		)
	case "removed":
		return handleInstallationRepositoriesRemoved(
			c,
			s,
			event.Installation.ID,
			event.RepositoriesRemoved,
			sesh,
		)
	default:
		sesh.Printf("unhandled event action: %s\n", event.Action)
		return nil
	}
}

func handleInstallationRepositoriesAdded(
	c *httpclient.Client, s *model.Store, ghInstallationID int64,
	repos []Repository, userID int32, email string, sesh *session.Session,
) error {
	sesh.Println("handling repositories added event...")

	for _, repo := range repos {
		_, err := s.CreateRepository(context.TODO(), model.CreateRepositoryParams{
			InstallationID: ghInstallationID,
			RepositoryID:   repo.ID,
			Name:           repo.Name,
			FullName:       repo.FullName,
			Url:            fmt.Sprintf("https://github.com/%s", repo.FullName),
			PathOnDisk: filepath.Join(
				config.Config.Progstack.RepositoriesPath,
				repo.FullName,
			),
		})
		if err != nil {
			/* XXX: cleanup delete from disk */
			return fmt.Errorf("error creating repository: %w", err)
		}
	}
	return nil
}

func handleInstallationRepositoriesRemoved(
	c *httpclient.Client, s *model.Store, ghInstallationID int64,
	repos []Repository, sesh *session.Session,
) error {
	sesh.Println("handling repositories removed event...")

	/* delete generated sites from disk, generated sites need subdomain */
	sesh.Println("deleting websites from disk...")
	for _, repo := range repos {
		path := fmt.Sprintf("%s/%s", config.Config.Progstack.WebsitesPath, repo.FullName)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("error deleting repo `%s' from disk: %w", path, err)
		}
	}
	/* delete repostories removed from disk */
	sesh.Println("deleting repositories from disk...")
	for _, repo := range repos {
		path := fmt.Sprintf("%s/%s", config.Config.Progstack.RepositoriesPath, repo.FullName)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("error deleting repo `%s' from disk: %w", path, err)
		}
	}
	/* delete blogs removed from db */
	sesh.Println("deleting blogs from db...")
	for _, repo := range repos {
		if err := s.DeleteRepositoryWithGhRepositoryID(context.TODO(), repo.ID); err != nil {
			return fmt.Errorf("error deleting repository with ghRepositoryID `%d': %w", repo.ID, err)
		}
	}
	return nil
}

func handlePush(
	c *httpclient.Client, s *model.Store,
	body []byte, sesh *session.Session,
) error {
	sesh.Println("handling push event...")

	var event PushEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("error unmarshaling push event: %w", err)
	}
	str, _ := eventToJSON(event)
	sesh.Printf("push event: %s\n", str)

	/* validate that blog exists for repository */
	b, err := s.GetBlogByGhRepositoryID(context.TODO(), event.Repository.ID)
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf(
				"error getting blog for repository event: %w",
				err,
			)
		}
		/* this can happen if user pushes to repo after installing
		* application without having created an associated blog*/
		sesh.Printf(
			"no associated blog with repositoryID `%d'\n",
			event.Repository.ID,
		)
		return nil
	}

	/* get branch for push event */
	branchName, err := getEventBranchName(event)
	if err != nil {
		return err
	}

	sesh.Printf("event branch: `%s'\n", branchName)
	sesh.Printf("live branch: `%s'\n", b.LiveBranch)

	if branchName != b.LiveBranch {
		/* event does not match live branch */
		return nil
	}

	if err := blog.UpdateRepositoryOnDisk(c, &b, sesh, s); err != nil {
		return fmt.Errorf("error pulling latest changes: %w", err)
	}
	return nil
}

func getEventBranchName(event PushEvent) (string, error) {
	if event.Ref != "" {
		refParts := strings.Split(event.Ref, "/")
		if len(refParts) > 2 && refParts[1] == "heads" {
			return strings.Join(refParts[2:], "/"), nil
		}
	}
	return "", fmt.Errorf("could not get extract branch name")
}
