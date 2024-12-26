package response

import (
	"net/http"
)

type empty struct{}

func NewEmpty() Response { return &empty{} }

func (e *empty) Respond(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
