package response

import (
	"encoding/json"
	"net/http"

	"github.com/hylodoc/hylodoc.com/internal/assert"
	"github.com/hylodoc/hylodoc.com/internal/session"
)

type jsonresponse struct {
	b []byte
}

func NewJson(data any) (Response, error) {
	b, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return &jsonresponse{b}, nil
}

func (resp *jsonresponse) Respond(w http.ResponseWriter, r *http.Request) error {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)
	sesh.Printf("response body: %s\n", string(resp.b))
	_, err := w.Write(resp.b)
	return err
}
