package installation

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

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
