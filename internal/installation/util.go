package installation

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/util"
)

func downloadRepoTarball(c *httpclient.Client, repoFullName, accessToken string) (string, error) {
	log.Println("downloading repo tarball...")

	/* build request */
	tarballUrl := fmt.Sprintf(ghRepositoriesTarballUrlTemplate, repoFullName)
	req, err := util.NewRequestBuilder("GET", tarballUrl).
		WithHeader("Authorization", fmt.Sprintf("Bearer %s", accessToken)).
		WithHeader("Accept", "application/vnd.github+json").
		Build()
	if err != nil {
		return "", err
	}

	/* make request */
	resp, err := c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download tar file: %s", resp.Status)
	}

	tmpSuffix := "*.tar.gz"
	tmpFile, err := os.CreateTemp("", tmpSuffix)
	if err != nil {
		return "", err
	}
	/* copy the response body to tmp file */
	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close() // Close on error to avoid resource leak
		return "", err
	}
	return tmpFile.Name(), nil
}

func extractTarball(tarPath, destPath string) error {
	log.Println("extracting tarball...")
	if err := os.MkdirAll(destPath, os.ModePerm); err != nil {
		log.Printf("error creating destination directory: %v\n", err)
		return fmt.Errorf("error creating destination directory: %w", err)
	}
	/* we assume that we're deployed on unix based compute, this is simpler
	* than in code go library to extract the Tar
	* --strip-components=1 ensures we place directly in destPath without
	* added dir*/
	cmd := exec.Command("tar", "--strip-components=1", "-xzf", tarPath,
		"-C", destPath)
	if err := cmd.Run(); err != nil {
		log.Printf("error extracting tar file: %v\n", err)
		return fmt.Errorf("error extraction tar file: %w", err)
	}
	return nil
}

/* validates that an event actually comes from our GithubApp by using the webhook
* secret configured on the GithubApp */
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
