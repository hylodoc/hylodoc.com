package request

import (
	"fmt"
	"log"
	"net/http"

	"github.com/xr0-org/progstack/internal/analytics"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/session"
)

type Request interface {
	Logger() *log.Logger
	Session() *session.Session
	MixpanelTrack(event string) error
}

type request struct {
	r *http.Request

	logger   *log.Logger
	sesh     *session.Session
	mixpanel *analytics.MixpanelClientWrapper
}

func NewRequest(r *http.Request) (Request, error) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		return nil, fmt.Errorf("no session")
	}
	return &request{
		r,
		logging.Logger(r),
		sesh,
		analytics.NewMixpanelClientWrapper(config.Config.Mixpanel.Token),
	}, nil
}

func (r *request) Logger() *log.Logger       { return r.logger }
func (r *request) Session() *session.Session { return r.sesh }
func (r *request) MixpanelTrack(event string) error {
	return r.mixpanel.Track(event, r.r)
}
