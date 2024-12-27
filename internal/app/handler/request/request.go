package request

import (
	"fmt"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"

	"github.com/gorilla/mux"
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
	ReadBody() ([]byte, error)

	MixpanelTrack(event string) error

	GetURLQueryValue(key string) string
	GetFormValue(name string) (string, error)
	GetPostFormValue(name string) (string, error)
	GetFormFile(name string) (multipart.File, *multipart.FileHeader, error)
	GetRouteVar(key string) (string, bool)
	GetHeader(name string) string
}

type request struct {
	r         *http.Request
	_readbody bool
	_body     []byte

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
		r, false, nil,
		w,
		logging.Logger(r),
		sesh,
		analytics.NewMixpanelClientWrapper(config.Config.Mixpanel.Token),
	}, nil
}

func (r *request) Logger() *log.Logger                 { return r.logger }
func (r *request) Session() *session.Session           { return r.sesh }
func (r *request) ResponseWriter() http.ResponseWriter { return r.w }

func (r *request) ReadBody() ([]byte, error) {
	if !r._readbody {
		body, err := ioutil.ReadAll(r.r.Body)
		if err != nil {
			return nil, err
		}
		r.Logger().Printf("request body: %s\n", string(body))
		defer r.r.Body.Close()
		r._body = body
		r._readbody = true
	}
	return r._body, nil
}

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

func (r *request) GetPostFormValue(name string) (string, error) {
	if r.r.Method != http.MethodPost {
		return "", fmt.Errorf("not POST")
	}
	return r.GetFormValue(name)
}

func (r *request) GetFormFile(
	name string,
) (multipart.File, *multipart.FileHeader, error) {
	/* XXX: add subscription based file size limits */
	const maxFileSize = 10 * 1024 * 1024 /* limit file size to 10MB */

	if err := r.r.ParseMultipartForm(maxFileSize); err != nil {
		return nil, nil, err
	}
	return r.r.FormFile(name)
}

func (r *request) GetRouteVar(key string) (string, bool) {
	v, ok := mux.Vars(r.r)[key]
	return v, ok
}

func (r *request) GetHeader(name string) string {
	v := r.r.Header.Get(name)
	r.Logger().Printf("%s: %s\n", name, v)
	return v
}
