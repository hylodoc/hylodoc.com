package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/authz"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

func Handle(r *mux.Router, pattern string, h handlerfunc) *mux.Route {
	return r.HandleFunc(
		pattern,
		func(w http.ResponseWriter, r *http.Request) {
			if err := execute(h, w, r); err != nil {
				sesh, ok := r.Context().Value(
					session.CtxSessionKey,
				).(*session.Session)
				assert.Printf(ok, "no session")

				/* TODO: error pages */
				if errors.Is(err, authz.SubscriptionError) {
					sesh.Println("authz error:", err)
					http.Error(
						w, "", http.StatusUnauthorized,
					)
					return
				}

				if err, ok := asCustomError(err); ok {
					sesh.Println("custom error:", err)
					http.Error(w, err.Error(), err.Code)
					return
				}

				sesh.Println("internal server error:", err)
				internalServerError(w, r)
			}
		},
	)
}

func asCustomError(err error) (*util.CustomError, bool) {
	var customErr *util.CustomError
	return customErr, errors.As(err, &customErr)
}

type handlerfunc func(request.Request) (response.Response, error)

func execute(h handlerfunc, w http.ResponseWriter, r *http.Request) error {
	sesh, ok := r.Context().Value(
		session.CtxSessionKey,
	).(*session.Session)
	assert.Printf(ok, "no session")

	resp, err := h(request.NewRequest(r, w, sesh))
	if err != nil {
		return fmt.Errorf("handler: %w", err)
	}
	if err := resp.Respond(w, r); err != nil {
		return fmt.Errorf("respond: %w", err)
	}
	return nil
}
