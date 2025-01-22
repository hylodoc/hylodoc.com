package analytics

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/mixpanel/mixpanel-go"
	"github.com/hylodoc/hylodoc.com/internal/assert"
	"github.com/hylodoc/hylodoc.com/internal/session"
)

type MixpanelClientWrapper struct {
	client *mixpanel.ApiClient
}

func NewMixpanelClientWrapper(token string) *MixpanelClientWrapper {
	return &MixpanelClientWrapper{
		client: mixpanel.NewApiClient(token),
	}
}

func (m *MixpanelClientWrapper) Track(event string, r *http.Request) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)
	go func() {
		if err := m.track(r, event); err != nil {
			sesh.Printf("Error emitting analytics: %v", err)
		}
	}()
}

func (m *MixpanelClientWrapper) track(r *http.Request, event string) error {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		return fmt.Errorf("No session for tracking")
	}

	identifiers := getIndentifiers(sesh)
	props := map[string]interface{}{
		"distinct_id": identifiers.distinctId,
		"ip":          getIP(r),
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

func getIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return forwarded
	}
	return r.RemoteAddr
}

type identifiers struct {
	distinctId string
	status     string
}

func getIndentifiers(sesh *session.Session) identifiers {
	if sesh.IsAuthenticated() {
		userid, err := sesh.GetUserID()
		assert.Assert(err == nil)
		return identifiers{
			distinctId: fmt.Sprintf("%s", userid),
			status:     "auth",
		}
	} else {
		return identifiers{
			distinctId: sesh.GetSessionID(),
			status:     "unauth",
		}
	}
}
