package response

import (
	"net/http"
)

type empty struct{}

func NewEmpty() Response { return &empty{} }

func (e *empty) Respond(w http.ResponseWriter, r *http.Request) error {
	w.WriteHeader(http.StatusOK)
	return nil
}
