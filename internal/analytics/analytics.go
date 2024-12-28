package analytics

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/mixpanel/mixpanel-go"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/session"
)

type MixpanelClientWrapper struct {
	client *mixpanel.ApiClient
}

func NewMixpanelClientWrapper(token string) *MixpanelClientWrapper {
	return &MixpanelClientWrapper{
		client: mixpanel.NewApiClient(token),
	}
}

func (m *MixpanelClientWrapper) Track(
	event string, r *http.Request, sesh *session.Session, logger *log.Logger,
) {
	go func() {
		if err := m.track(event, r, sesh, logger); err != nil {
			logger.Printf("Error emitting analytics: %v", err)
		}
	}()
}

func (m *MixpanelClientWrapper) track(
	event string, r *http.Request, sesh *session.Session, logger *log.Logger,
) error {
	ip := r.Header.Get("X-Forwarded-For")
	identifiers := getIndentifiers(sesh)
	props := map[string]interface{}{
		"distinct_id": identifiers.distinctId,
		"ip":          ip,
		"url":         r.URL.String(),
		"time":        time.Now().Unix(),
		"status":      identifiers.status,
		"$insert_id":  uuid.New().String(),
	}

	if err := m.client.Track(
		context.TODO(),
		[]*mixpanel.Event{m.client.NewEvent(
			event,
			identifiers.distinctId,
			props,
		)},
	); err != nil {
		return fmt.Errorf("Error calling mixpanel: %w", err)
	}
	return nil
}

type identifiers struct {
	distinctId string
	status     string
}

func getIndentifiers(sesh *session.Session) identifiers {
	if sesh.IsUnauth() {
		return identifiers{
			distinctId: sesh.GetSessionID(),
			status:     "unauth",
		}
	}
	userid, err := sesh.GetUserID()
	assert.Assert(err == nil)
	return identifiers{
		distinctId: fmt.Sprintf("%d", userid),
		status:     "auth",
	}
}
