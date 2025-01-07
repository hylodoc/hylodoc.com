package blog

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/util"
)

const (
	ghRepositoriesTarballUrlTemplate = "https://api.github.com/repos/%s/tarball/%s"
)

func downloadRepoTarball(
	c *httpclient.Client, repoFullName, branch, accessToken string,
) (string, error) {
	log.Println("downloading repo tarball...")

	/* build request */
	tarballUrl := buildTarballUrl(repoFullName, branch)
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

func buildTarballUrl(repoFullName, branch string) string {
	return fmt.Sprintf(ghRepositoriesTarballUrlTemplate, repoFullName, branch)
}

func extractTarball(src, dst string) error {
	/* remove dst dir */
	if _, err := os.Stat(dst); err == nil {
		if err := os.RemoveAll(dst); err != nil {
			return fmt.Errorf("error removing destination directory: %w", err)
		}
	}

	/* create fresh dst dir */
	if err := os.MkdirAll(dst, os.ModePerm); err != nil {
		return fmt.Errorf("error creating destination directory: %w", err)
	}

	/* we assume that we're deployed on unix based compute, this is simpler
	* than in code go library to extract the Tar
	* --strip-components=1 ensures we place directly in destPath without
	* added dir*/

	/* XXX: fix this to be platform independent (See how we handle .zip
	* elsewhere ) */
	cmd := exec.Command("tar", "--strip-components=1", "-xzf", src, "-C", dst)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error extraction tar file: %w", err)
	}
	return nil
}
