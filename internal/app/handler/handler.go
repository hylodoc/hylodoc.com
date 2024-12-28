package handler

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/authz"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

type Handler struct {
	s *model.Store
}

func NewHandler(s *model.Store) *Handler { return &Handler{s} }

func (h *Handler) Handle(
	r *mux.Router, pattern string, f handlerfunc,
) *mux.Route {
	return r.HandleFunc(
		pattern,
		func(w http.ResponseWriter, r *http.Request) {
			logger := logging.NewLogger()
			if err := execute(f, w, r, logger, h.s); err != nil {
				/* TODO: error pages */
				if errors.Is(err, authz.SubscriptionError) {
					logger.Println("authz error:", err)
					http.Error(
						w, "", http.StatusUnauthorized,
					)
					return
				}

				if err, ok := asCustomError(err); ok {
					logger.Println("custom error:", err)
					http.Error(w, err.Error(), err.Code)
					return
				}

				logger.Println("internal server error:", err)
				http.Error(
					w, "", http.StatusInternalServerError,
				)
			}
		},
	)
}

func asCustomError(err error) (*util.CustomError, bool) {
	var customErr *util.CustomError
	return customErr, errors.As(err, &customErr)
}

type handlerfunc func(request.Request) (response.Response, error)

func execute(
	f handlerfunc,
	w http.ResponseWriter, r *http.Request,
	logger *log.Logger,
	s *model.Store,
) error {
	sesh, err := session.NewSession(w, r, logger, s)
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	resp, err := f(request.NewRequest(r, w, sesh, logger))
	if err != nil {
		return fmt.Errorf("handler func: %w", err)
	}
	resp.Respond(w, r)
	return nil
}
