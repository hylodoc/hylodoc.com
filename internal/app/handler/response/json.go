package response

import (
	"encoding/json"
	"net/http"
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

func (resp *jsonresponse) Respond(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp.b)
}
