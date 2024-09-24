package installation

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/xr0-org/progstack/internal/util"
)

func downloadRepoTarball(c *http.Client, repoFullName, accessToken string) (string, error) {
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
		fmt.Println("error creating destination directory: %w", err)
		return fmt.Errorf("error creating destination directory: %w", err)
	}
	/* we assume that we're deployed on unix based compute, this is simpler
	* than in code go library to extract the Tar
	* --strip-components=1 ensures we place directly in destPath without
	* added dir*/
	cmd := exec.Command("tar", "--strip-components=1", "-xzf", tarPath,
		"-C", destPath)
	if err := cmd.Run(); err != nil {
		log.Fatalf("error extracting tar file: %v", err)
		return fmt.Errorf("error extraction tar file: %w", err)
	}
	return nil
}
