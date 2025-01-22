package installation

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/hylodoc/hylodoc.com/internal/app/handler/request"
)

/* validates that an event actually comes from our GithubApp by using the webhook
* secret configured on the GithubApp */
func validateSignature(r request.Request, secret string) error {
	body, err := r.ReadBody()
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	signature := r.GetHeader("X-Hub-Signature-256")
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
		return fmt.Errorf(
			"expected signature: %s but got: %s",
			expectedSignature, signature,
		)
	}
	return nil
}
