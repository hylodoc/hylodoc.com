package analytics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/mixpanel/mixpanel-go"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/session"
)

type AnalyticsService struct {
	client *mixpanel.ApiClient
}

func NewMixpanelClient(token string) *mixpanel.ApiClient {
	return mixpanel.NewApiClient(token)
}

func NewAnalyticsService(c *mixpanel.ApiClient) *AnalyticsService {
	return &AnalyticsService{
		client: c,
	}
}

func (m *AnalyticsService) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logging.Logger(r)

		ip := r.Header.Get("X-Forwarded-For")
		if ip == "" {
			next.ServeHTTP(w, r)
		}
		hashedIp, err := hashIp(ip)
		if err != nil {
			logger.Println("Could not hash ip", err)
			http.Error(w, "", http.StatusInternalServerError)
		}

		sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
		if !ok {
			logger.Println("No session")
			http.Error(w, "", http.StatusInternalServerError)
		}

		identifiers := getIndentifiers(sesh)
		base_props := map[string]interface{}{
			"ip":          hashedIp,
			"time":        time.Now().Unix(),
			"distinct_id": identifiers.distinctId,
			"status":      identifiers.status,
			"$insert_id":  uuid.New().String(),
		}

		if err := m.client.Track(
			context.TODO(),
			[]*mixpanel.Event{m.client.NewEvent(
				r.URL.String(),
				uuid.New().String(),
				base_props,
			)},
		); err != nil {
			logger.Printf("Error calling mixpanel: %v", err)
			/* XXX: should not fail requests on failing to write
			* metrics */
		}

		next.ServeHTTP(w, r)
	})
}

type identifiers struct {
	distinctId string
	status     string
}

func getIndentifiers(sesh *session.Session) identifiers {
	if sesh.IsAuthenticated() {
		return identifiers{
			distinctId: fmt.Sprintf("%d", sesh.GetUserID()),
			status:     "auth",
		}
	} else {
		return identifiers{
			distinctId: sesh.GetSessionID(),
			status:     "unauth",
		}
	}
}

func hashIp(ip string) (string, error) {
	parsedIp := net.ParseIP(ip)
	if parsedIp == nil {
		return "", fmt.Errorf("invalid IP address: %s", ip)
	}

	hasher := sha256.New()

	_, err := hasher.Write(parsedIp)
	if err != nil {
		return "", err
	}

	hashBytes := hasher.Sum(nil)
	hashString := hex.EncodeToString(hashBytes)

	return hashString, nil
}
