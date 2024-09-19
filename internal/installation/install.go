package installation

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/auth"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/util"
)

const (
	ghRepositoriesUrl = "https://api.github.com/installation/repositories"

	ghInstallUrlTemplate             = "https://github.com/apps/%s/installations/new"
	ghAccessTokenUrlTemplate         = "https://api.github.com/app/installations/%d/access_tokens"
	ghRepositoriesTarballUrlTemplate = "https://api.github.com/repos/%s/%s/tarball"
)

type InstallationService struct {
	client *http.Client
	store  *model.Store
	config *config.Configuration
}

func NewInstallationService(c *http.Client, s *model.Store, config *config.Configuration) *InstallationService {
	return &InstallationService{
		client: c,
		store:  s,
		config: config,
	}
}

func validateSignature(r *http.Request, secret string) error {
	log.Println("validating github signature...")
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("error reading body: %w", err)
	}
	defer r.Body.Close()

	/* place back in request */
	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))

	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		return fmt.Errorf("missing X-Hub-Signature-256 header")
	}

	/* create a new HMAC using SHA-256 and the provided secret */
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expectedMAC := mac.Sum(nil)

	/* compare the received signature with the computed one */
	expectedSignature := "sha256=" + hex.EncodeToString(expectedMAC)
	ok := hmac.Equal([]byte(signature), []byte(expectedSignature))
	if !ok {
		return fmt.Errorf("expected signature: %s but got: %s", err)
	}
	return nil
}

func (i *InstallationService) InstallationCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		/* XXX: metrics */

		err := i.installationCallback(w, r)
		if err != nil {
			log.Printf("error in installation callback: %v", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/home", http.StatusTemporaryRedirect)
	}
}

func (i *InstallationService) installationCallback(w http.ResponseWriter, r *http.Request) error {
	log.Println("installationCallback handler...")

	/* validate authenticity using Github webhook secret */
	if err := validateSignature(r, i.config.Github.WebhookSecret); err != nil {
		return fmt.Errorf("error validating github signature: %w", err)
	}

	/* XXX: validate that user actually called */
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("error reading request body: %w", err)
	}
	var payload InstallationPayload
	if err = json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("error unmarshalling JSON: %w", err)
	}
	/* XXX: is this safe given that we validated the request with the
	* webhook? */
	user, err := i.store.GetUserByGithubId(context.TODO(), payload.Sender.ID)
	if err != nil {
		return fmt.Errorf("error getting user from id in event: %w", err)
	}

	/* handle kinds of events */
	eventType := r.Header.Get("X-GitHub-Event")
	switch eventType {
	case "installation":
		/* XXX: userID hardcoded right now */
		return handleInstallation(i.client, i.store, body, user.ID)
	case "installation_repositories":
		return handleInstallationRepositories(i.client, i.store, body)
	case "push":
		return handlePush(i.client, i.store, body)
	default:
		log.Println("unhandled event type: %s", eventType)
	}
	return nil
}

func handleInstallation(c *http.Client, s *model.Store, body []byte, userID int32) error {
	log.Println("handling installation event...")
	var event InstallationPayload
	if err := json.Unmarshal(body, &event); err != nil {
		return err
	}
	str, _ := eventToJSON(event)
	log.Println("installation event: %s", str)

	switch event.Action {
	case "created":
		return handleInstallationCreated(c, s, &event.Installation, userID)
	case "deleted":
		return handleInstallationDeleted(c, s, &event.Installation, userID)
	default:
		log.Println("unhandled event action: %s", event.Action)
	}

	return nil
}

func handleInstallationCreated(c *http.Client, s *model.Store, i *Installation, userID int32) error {
	log.Println("handling installation created event...")
	/* get access token */
	accessToken, err := auth.GetInstallationAccessToken(
		c,
		config.Config.Github.AppID,
		i.ID,
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
	if err = downloadReposToDisk(c, repos, accessToken); err != nil {
		return fmt.Errorf("error downloading repos to disk: %w", err)
	}

	/* write installation and repos to db Tx */
	createInstallationTxParams := buildCreateInstallationTxParams(i.ID, userID, repos)
	if err = s.CreateInstallationTx(context.TODO(), createInstallationTxParams); err != nil {
		/* XXX: wipe relavant repos from disk */
		return fmt.Errorf("error executing db transaction: %w", err)
	}
	return nil
}

func buildCreateInstallationTxParams(installationID int64, userID int32, repos []Repository) model.InstallationTxParams {
	var iTxParams model.InstallationTxParams
	iTxParams.InstallationID = installationID
	iTxParams.UserID = userID
	var reposTxParams []model.RepositoryTxParams
	for _, repo := range repos {
		reposTxParams = append(reposTxParams, model.RepositoryTxParams{
			GhRepositoryID: repo.ID,
			Name:           repo.Name,
			Url:            repo.HtmlUrl,
			Owner:          repo.Owner.Login,
		})
	}
	iTxParams.Repositories = reposTxParams
	return iTxParams
}

func getReposDetails(c *http.Client, accessToken string) ([]Repository, error) {
	log.Println("getting repositories details...")
	req, err := util.NewRequestBuilder("GET", ghRepositoriesUrl).
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

	var reposResponse RepositoriesResponse
	if err := json.Unmarshal(body, &reposResponse); err != nil {
		return []Repository{}, err
	}
	return reposResponse.Repositories, nil
}

func downloadReposToDisk(c *http.Client, repos []Repository, accessToken string) error {
	log.Println("downloading repos to disk...")
	for _, repo := range repos {
		owner := repo.Owner.Login
		name := repo.Name
		/* download tarball and write to tmp file */
		tmpFile, err := downloadRepoTarball(c, owner, name, accessToken)
		if err != nil {
			return fmt.Errorf("error downloading tarball for /%s/%s: %w", owner, name, err)
		}
		/* extract tarball to destination */
		tmpDst := fmt.Sprintf("%s/%s/%s", config.Config.Progstack.RepositoriesPath, owner, name)
		if err = extractTarball(tmpFile, tmpDst); err != nil {
			return fmt.Errorf("error extracting tarball to destination for /%s/%s: %w", owner, name, err)
		}
	}
	return nil
}

func handleInstallationDeleted(c *http.Client, s *model.Store, installation *Installation, userID int32) error {
	log.Println("handling installation deleted event...")

	/* fetch repos associated with installation */
	log.Printf("deleting installation %d for user %d...", installation.ID, userID)
	repos, err := s.GetRepositoriesForInstallation(context.TODO(), installation.ID)
	if err != nil {
		return fmt.Errorf("error getting repositories for installation %d: %w", installation.ID, err)
	}
	/* delete the repos on disk */
	if err = deleteReposFromDisk(repos); err != nil {
		return fmt.Errorf("error deleting repos from disk")
	}
	/* cascade delete the installation and associated repos */
	if err = s.DeleteInstallationWithGithubId(context.TODO(), installation.ID); err != nil {
		return fmt.Errorf("error deleting installation: %w", err)
	}
	return nil
}

func deleteReposFromDisk(repos []model.GetRepositoriesForInstallationRow) error {
	log.Println("deleting repos from disk...")
	for _, repo := range repos {
		path := fmt.Sprintf("%s/%s/%s", config.Config.Progstack.RepositoriesPath, repo.Owner, repo.Name)
		log.Printf("deleting repo at %s from disk...\n", path)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("error deleting repo %s from disk: %w", err)
		}
	}
	return nil
}

func handleInstallationRepositories(c *http.Client, s *model.Store, body []byte) error {
	log.Println("handling installation repositories event...")

	/* XXX: handle different actions */
	assert.Printf(false, "handleInstallationRepositories not implemented")

	return nil
}

func handlePush(c *http.Client, s *model.Store, body []byte) error {
	log.Println("handling push event...")

	/* XXX: validate and update repo on disk */
	assert.Printf(false, "handlePush not implemented")

	return nil
}
