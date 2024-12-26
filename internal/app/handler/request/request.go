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

	/* TODO: refactor authn session handling to remove this */
	ResponseWriter() http.ResponseWriter

	MixpanelTrack(event string) error

	GetURLQueryValue(key string) string
	GetFormValue(name string) (string, error)
}

type request struct {
	r *http.Request
	w http.ResponseWriter

	logger   *log.Logger
	sesh     *session.Session
	mixpanel *analytics.MixpanelClientWrapper
}

func NewRequest(r *http.Request, w http.ResponseWriter) (Request, error) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	if !ok {
		return nil, fmt.Errorf("no session")
	}
	return &request{
		r, w,
		logging.Logger(r),
		sesh,
		analytics.NewMixpanelClientWrapper(config.Config.Mixpanel.Token),
	}, nil
}

func (r *request) Logger() *log.Logger                 { return r.logger }
func (r *request) Session() *session.Session           { return r.sesh }
func (r *request) ResponseWriter() http.ResponseWriter { return r.w }

func (r *request) MixpanelTrack(event string) error {
	return r.mixpanel.Track(event, r.r)
}

func (r *request) GetURLQueryValue(key string) string {
	return r.r.URL.Query().Get(key)
}

func (r *request) GetFormValue(name string) (string, error) {
	if err := r.r.ParseForm(); err != nil {
		return "", err
	}
	return r.r.FormValue(name), nil
}
