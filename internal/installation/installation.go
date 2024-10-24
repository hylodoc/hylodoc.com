package installation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/resend/resend-go/v2"
	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/email"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/util"
)

const (
	GhInstallUrlTemplate = "https://github.com/apps/%s/installations/new"

	ghInstallationRepositoriesUrl    = "https://api.github.com/installation/repositories"
	ghAccessTokenUrlTemplate         = "https://api.github.com/app/installations/%d/access_tokens"
	ghRepositoriesTarballUrlTemplate = "https://api.github.com/repos/%s/tarball"
)

type InstallationService struct {
	client       *httpclient.Client
	resendClient *resend.Client
	store        *model.Store
	config       *config.Configuration
}

func NewInstallationService(c *httpclient.Client, r *resend.Client, s *model.Store, config *config.Configuration) *InstallationService {
	return &InstallationService{
		client:       c,
		resendClient: r,
		store:        s,
		config:       config,
	}
}

func (i *InstallationService) InstallationCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		/* XXX: metrics */

		if err := i.installationCallback(w, r); err != nil {
			log.Printf("error in installation callback: %v\n", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

func (i *InstallationService) installationCallback(w http.ResponseWriter, r *http.Request) error {
	log.Println("installationCallback handler...")

	/* validate authenticity using Github webhook secret */
	if err := validateSignature(r, i.config.Github.WebhookSecret); err != nil {
		return fmt.Errorf("error validating github signature: %w", err)
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("error reading request body: %w", err)
	}

	/* handle kinds of events */
	eventType := r.Header.Get("X-GitHub-Event")
	switch eventType {
	case "installation":
		return handleInstallation(i.client, i.store, body)
	case "installation_repositories":
		return handleInstallationRepositories(i.client, i.store, body)
	case "push":
		return handlePush(i.client, i.resendClient, i.store, body)
	default:
		log.Println("unhandled event type: %s", eventType)
	}
	return nil
}

func handleInstallation(c *httpclient.Client, s *model.Store, body []byte) error {
	log.Println("handling installation event...")

	var event InstallationEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return err
	}
	str, _ := eventToJSON(event)
	log.Println("installation event: %s", str)

	/* XXX: is this safe given that we validated the request with the
	* webhook? */
	ghUserID := event.Sender.ID
	user, err := s.GetUserByGhUserID(context.TODO(), ghUserID)
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("error getting user with ghUserID `%s' (in event) from db: %w", ghUserID, err)
		}
	}

	ghInstallationID := event.Installation.ID
	switch event.Action {
	case "created":
		return handleInstallationCreated(c, s, ghInstallationID, user.ID, user.GhEmail)
	case "deleted":
		return handleInstallationDeleted(c, s, ghInstallationID, user.ID)
	default:
		log.Println("unhandled event action: %s", event.Action)
	}

	return nil
}

func handleInstallationCreated(c *httpclient.Client, s *model.Store, ghInstallationID int64, userID int32, ghEmail string) error {
	log.Println("handling installation created event...")
	/* get access token */
	accessToken, err := auth.GetInstallationAccessToken(
		c,
		config.Config.Github.AppID,
		ghInstallationID,
		config.Config.Github.PrivateKeyPath,
	)
	if err != nil {
		return fmt.Errorf("error getting installation access token: %w", err)
	}

	/* get repositories from Github */
	repos, err := getReposDetails(c, accessToken)
	if err != nil {
		return fmt.Errorf("error getting repositories: %w", err)
	}
	str, _ := eventToJSON(repos)
	log.Println("repos: %s", str)

	/* clone all repos to disk */
	if err = downloadReposToDisk(c, repos, userID, accessToken); err != nil {
		return fmt.Errorf("error downloading repos to disk: %w", err)
	}

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
	return iTxParams
}

type InstallationRepositoriesResponse struct {
	TotalCount   int          `json:"total_count"`
	Repositories []Repository `json:"repositories"` /* XXX: reusing from events */
}

func getReposDetails(c *httpclient.Client, accessToken string) ([]Repository, error) {
	log.Println("getting repositories details...")
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
		log.Println("error unpacking body: ", err)
		return []Repository{}, err
	}
	log.Printf("body: %s", body)

	var reposResponse InstallationRepositoriesResponse
	if err := json.Unmarshal(body, &reposResponse); err != nil {
		return []Repository{}, err
	}
	return reposResponse.Repositories, nil
}

/* this downloads all the repositories to disk int tmp folder then extracts them
* to the configured destination on disk
*
* NOTE: we use the Repository.GhFullName which is guaranteed to be
* "<owner>/<name>" to build the repo path */
func downloadReposToDisk(c *httpclient.Client, repos []Repository, userID int32, accessToken string) error {
	log.Println("downloading repos to disk...")
	for _, repo := range repos {
		/* download tarball and write to tmp file */
		repoFullName := repo.FullName
		tmpFile, err := downloadRepoTarball(c, repoFullName, accessToken)
		if err != nil {
			return fmt.Errorf("error downloading tarball for at url: %s: %w", repoFullName, err)
		}
		/* extract tarball to destination should store under user */
		dst := fmt.Sprintf("%s/%d/%s", config.Config.Progstack.RepositoriesPath, userID, repoFullName)
		log.Printf("repo dst: %s\n", dst)
		if err = extractTarball(tmpFile, dst); err != nil {
			return fmt.Errorf("error extracting tarball to destination for /%s/: %w", repoFullName, err)
		}
	}
	return nil
}

func handleInstallationDeleted(c *httpclient.Client, s *model.Store, ghInstallationID int64, userID int32) error {
	log.Println("handling installation deleted event...")

	/* fetch repos associated with installation */
	log.Printf("deleting installation %d for user %d...", ghInstallationID, userID)
	repos, err := s.ListBlogsForInstallationByGhInstallationID(context.TODO(), ghInstallationID)
	if err != nil {
		return fmt.Errorf("error getting repositories for installation %d: %w", ghInstallationID, err)
	}
	/* delete the repos on disk */
	log.Println("deleting repos from disk...")
	for _, repo := range repos {
		path := fmt.Sprintf("%s/%d/%s", config.Config.Progstack.RepositoriesPath, userID, repo.FullName)
		log.Printf("deleting repo at `%s' from disk...\n", path)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("error deleting repo `%s' from disk: %w", err)
		}
	}
	/* delete generated websites */
	log.Println("deleting generated websites from disk...")
	for _, repo := range repos {
		path := fmt.Sprintf("%s/%s", config.Config.Progstack.WebsitesPath, repo.Subdomain)
		log.Printf("deleting website at `%s' from disk...\n", path)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("error deleting website `%s' from disk: %w", repo.Subdomain, err)
		}
	}
	/* cascade delete the installation and associated repos */
	log.Println("deleting installation with ghInstallationID `%d'...", ghInstallationID)
	if err = s.DeleteInstallationWithGithubInstallationID(context.TODO(), ghInstallationID); err != nil {
		return fmt.Errorf("error deleting installation: %w", err)
	}
	return nil
}

func handleInstallationRepositories(c *httpclient.Client, s *model.Store, body []byte) error {
	log.Println("handling installation repositories event...")

	var event InstallationRepositoriesEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("error unmarshaling InstallationRepositoriesEvent: %w", err)
	}
	str, _ := eventToJSON(event)
	log.Printf("installationRepositoriesEvent: %s", str)

	ghUserID := event.Sender.ID
	user, err := s.GetUserByGhUserID(context.TODO(), ghUserID)
	if err != nil {
		if err != sql.ErrNoRows {
			return fmt.Errorf("error getting user with ghUserID `%s' (in event) from db: %w", ghUserID, err)
		}
	}
	ghInstallationID := event.Installation.ID

	/* check that installation exists */
	_, err = s.GetInstallationByGithubInstallationID(context.TODO(), ghInstallationID)
	if err != nil {
		return fmt.Errorf("error getting installation with ghInstallationID: %w", err)
	}

	switch event.Action {
	case "added":
		return handleInstallationRepositoriesAdded(c, s, ghInstallationID, event.RepositoriesAdded, user.ID, user.GhEmail)
	case "removed":
		return handleInstallationRepositoriesRemoved(c, s, ghInstallationID, event.RepositoriesRemoved)
	default:
		log.Println("unhandled event action: %s", event.Action)
	}

	return nil
}

func handleInstallationRepositoriesAdded(c *httpclient.Client, s *model.Store, ghInstallationID int64, repos []Repository, userID int32, email string) error {
	log.Println("handling repositories added event...")

	/* get access token */
	accessToken, err := auth.GetInstallationAccessToken(
		c,
		config.Config.Github.AppID,
		ghInstallationID,
		config.Config.Github.PrivateKeyPath,
	)
	if err != nil {
		return fmt.Errorf("error getting installation access token: %w", err)
	}
	/* clone respositories added to disk */
	if err = downloadReposToDisk(c, repos, userID, accessToken); err != nil {
		return fmt.Errorf("error downloading repos to disk: %w", err)
	}

	for _, repo := range repos {
		_, err := s.CreateRepository(context.TODO(), model.CreateRepositoryParams{
			InstallationID: ghInstallationID,
			RepositoryID:   repo.ID,
			Name:           repo.Name,
			FullName:       repo.FullName,
			Url:            fmt.Sprintf("https://github.com/%s", repo.FullName),
		})
		if err != nil {
			/* XXX: cleanup delete from disk */
			return fmt.Errorf("error creating repository: %w", err)
		}
	}
	return nil
}

func handleInstallationRepositoriesRemoved(c *httpclient.Client, s *model.Store, ghInstallationID int64, repos []Repository) error {
	log.Println("handling repositories removed event...")

	log.Println("deleting websites from disk...")
	/* delete generated sites from disk, generated sites need subdomain */
	for _, repo := range repos {
		path := fmt.Sprintf("%s/%s", config.Config.Progstack.WebsitesPath, repo.FullName)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("error deleting repo `%s' from disk: %w", path, err)
		}
	}
	log.Println("deleting repositories from disk...")
	/* delete repostories removed from disk */
	for _, repo := range repos {
		path := fmt.Sprintf("%s/%s", config.Config.Progstack.RepositoriesPath, repo.FullName)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("error deleting repo `%s' from disk: %w", path, err)
		}
	}
	log.Println("deleting blogs from db...")
	/* delete blogs removed from db */
	for _, repo := range repos {
		if err := s.DeleteRepositoryWithGhRepositoryID(context.TODO(), repo.ID); err != nil {
			return fmt.Errorf("error deleting repository with ghRepositoryID `%d': %w", repo.ID, err)
		}
	}
	return nil
}

func handlePush(c *httpclient.Client, resendClient *resend.Client, s *model.Store, body []byte) error {
	log.Println("handling push event...")

	var event PushEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return fmt.Errorf("error unmarshaling push event: %w", err)
	}
	str, _ := eventToJSON(event)
	log.Println("push event: %s", str)

	/* XXX: validate that installation and repository exists locally, i.e.
	* we are in sync */

	/* get access token */
	ghInstallationID := event.Installation.ID
	accessToken, err := auth.GetInstallationAccessToken(
		c,
		config.Config.Github.AppID,
		ghInstallationID,
		config.Config.Github.PrivateKeyPath,
	)
	if err != nil {
		return fmt.Errorf("error getting installation access token: %w", err)
	}

	/* download repo tarball */
	repoFullName := event.Repository.FullName
	tmpFile, err := downloadRepoTarball(c, repoFullName, accessToken)
	if err != nil {
		return fmt.Errorf("error downloading tarball for at url: %s: %w", repoFullName, err)
	}

	/* extract tarball to destination should store under user */
	tmpDst := fmt.Sprintf("%s/%s", config.Config.Progstack.RepositoriesPath, repoFullName)
	if err = extractTarball(tmpFile, tmpDst); err != nil {
		return fmt.Errorf("error extracting tarball to destination for /%s/: %w", repoFullName, err)
	}

	/* XXX: generate website afresh */

	/* XXX: users should trigger emails manually on dashboard? how do we
	* ensure that we send emails for the right post, since the posts are
	* generated by the repository */
	ghRepositoryID := event.Repository.ID
	if err = sendNewPostUpdateEmailsForBlog(ghRepositoryID, resendClient, s); err != nil {
		return fmt.Errorf("error sending post emails for repo `%d': %w", ghRepositoryID, err)
	}
	return nil
}

func sendNewPostUpdateEmailsForBlog(ghRepositoryID int64, c *resend.Client, s *model.Store) error {
	/* build parameters */
	paramsList, err := buildNewPostUpdateParamsList(ghRepositoryID, s)
	if err != nil {
		return fmt.Errorf("error building newPostUpdatesParamsList: %w", err)
	}

	/* send emails for all parameter */
	for _, params := range paramsList {
		err := email.SendNewPostUpdate(c, params)
		if err != nil {
			return fmt.Errorf("error sending new post update email: %w", err)
		}
	}
	return nil
}

func buildNewPostUpdateParamsList(ghRepositoryID int64, s *model.Store) ([]email.NewPostUpdateParams, error) {
	/* get blog */
	blog, err := s.GetBlogByGhRepositoryID(context.TODO(), sql.NullInt64{
		Valid: true,
		Int64: ghRepositoryID,
	})
	if err != nil {
		return []email.NewPostUpdateParams{}, fmt.Errorf("error getting blog with ghRepositoryID `%d': %w", ghRepositoryID, err)
	}
	log.Printf("sending new post update emails for blog with id: `%d'\n", blog.ID)

	/* list active subscribers */
	subscribers, err := s.ListActiveSubscribersForGhRepositoryID(context.TODO(), sql.NullInt64{
		Valid: true,
		Int64: ghRepositoryID,
	})
	if err != nil {
		return []email.NewPostUpdateParams{}, fmt.Errorf("error getting active subscriber list: %w", err)
	}

	/* build details */
	var paramsList []email.NewPostUpdateParams
	blogParams := email.BlogParams{
		ID:        blog.ID,
		From:      blog.FromAddress,
		Subdomain: blog.Subdomain,
	}
	/* XXX: construct postParams from generated site, hardcoding for now */
	postParams := email.PostParams{
		Link:    fmt.Sprintf("https://%s.progstack.com/posts/1", blog.Subdomain),
		Body:    "testing subscriber update emails in progstack",
		Subject: "#1 progstack email functinality",
	}
	/* send each subscriber an email */
	/* XXX: should prolly use their bulk API, does up to 100 per batch */
	for _, subscriber := range subscribers {
		subscriberParams := email.SubscriberParams{
			To:               subscriber.Email,
			UnsubscribeToken: subscriber.UnsubscribeToken.String(),
		}
		log.Printf("sending email to subscriber: `%s'\n", subscriberParams.To)

		paramsList = append(paramsList, email.NewPostUpdateParams{
			Blog:       blogParams,
			Subscriber: subscriberParams,
			Post:       postParams,
		})
	}
	return paramsList, nil
}
