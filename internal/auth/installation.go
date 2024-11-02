package auth

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/xr0-org/progstack/internal/httpclient"
	"github.com/xr0-org/progstack/internal/util"
)

/* Installation AccessToken */

type InstallationAccessToken struct {
	Token     string    `json:"token"`      /* The installation access token */
	ExpiresAt time.Time `json:"expires_at"` /* The timestamp when the token expires */
}

func GetInstallationAccessToken(
	client *httpclient.Client, appID, installationID int64,
	privateKeyPath string,
) (string, error) {
	jwt, err := createJWT(appID, privateKeyPath)
	if err != nil {
		return "", fmt.Errorf("Error creating JWT: %w", err)
	}

	url := fmt.Sprintf(ghInstallationAccessTokenUrlTemplate, installationID)
	req, err := util.NewRequestBuilder("POST", url).
		WithHeader("Authorization", fmt.Sprintf("Bearer %s", jwt)).
		WithHeader("Accept", "application/vnd.github+json").
		WithHeader("X-GitHub-Api-Version", "2022-11-28").
		Build()
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var accessToken InstallationAccessToken
	err = json.Unmarshal(body, &accessToken)
	if err != nil {
		return "", err
	}
	return accessToken.Token, nil
}

func createJWT(appID int64, privateKeyPath string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Add(-60 * time.Second).Unix(), /* allow for clock drift per docs */
		"exp": now.Add(10 * time.Minute).Unix(),
		"iss": appID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	privateKey, err := loadPrivateKey(privateKeyPath)
	if err != nil {
		return "", fmt.Errorf("Error loading private key: %w", err)
	}
	jwtToken, err := token.SignedString(privateKey)
	if err != nil {
		return "", fmt.Errorf("error signing token with private key: %w", err)
	}
	err = validateJWT(jwtToken, &privateKey.PublicKey)
	if err != nil {
		return "", fmt.Errorf("error validating jwt token: %w", err)
	}
	return jwtToken, nil
}

func loadPrivateKey(filePath string) (*rsa.PrivateKey, error) {
	keyData, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("error parsing private key: %w", err)
	}
	return jwt.ParseRSAPrivateKeyFromPEM(keyData)
}

func validateJWT(tokenString string, publicKey *rsa.PublicKey) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Ensure the signing method is RS256
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return publicKey, nil
	})
	if err != nil {
		return err
	}
	if !token.Valid {
		return fmt.Errorf("invalid token")
	}
	return nil
}
