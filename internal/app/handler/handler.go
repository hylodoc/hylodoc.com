package handler

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/knuthic/knuthic/internal/app/handler/request"
	"github.com/knuthic/knuthic/internal/app/handler/response"
	"github.com/knuthic/knuthic/internal/assert"
	"github.com/knuthic/knuthic/internal/session"
)

func Handle(r *mux.Router, pattern string, h handlerfunc) *mux.Route {
	return r.HandleFunc(
		pattern,
		func(w http.ResponseWriter, r *http.Request) {
			if err := execute(h, w, r); err != nil {
				HandleError(w, r, err)
			}
		},
	)
}

type handlerfunc func(request.Request) (response.Response, error)

func execute(h handlerfunc, w http.ResponseWriter, r *http.Request) error {
	sesh, ok := r.Context().Value(
		session.CtxSessionKey,
	).(*session.Session)
	assert.Assert(ok)

	resp, err := h(request.NewRequest(r, w, sesh))
	if err != nil {
		return fmt.Errorf("handler: %w", err)
	}
	if err := resp.Respond(w, r); err != nil {
		return fmt.Errorf("respond: %w", err)
	}
	return nil
}
