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
	"github.com/xr0-org/progstack/internal/util"
)

func Handle(r *mux.Router, pattern string, h handlerfunc) *mux.Route {
	return r.HandleFunc(
		pattern,
		func(w http.ResponseWriter, r *http.Request) {
			if err := execute(h, w, r); err != nil {
				handleError(err, w, logging.Logger(r))
			}
		},
	)
}

type handlerfunc func(request.Request) (response.Response, error)

func handleError(err error, w http.ResponseWriter, logger *log.Logger) {
	/* TODO: error page */
	if errors.Is(err, authz.SubscriptionError) {
		logger.Println("authz error", err)
		http.Error(w, "", http.StatusUnauthorized)
	} else if err, ok := asCustomError(err); ok {
		logger.Println("custom error", err)
		http.Error(w, err.Error(), err.Code)
	} else {
		logger.Println("AsHttp error", err)
		http.Error(w, "", http.StatusInternalServerError)
	}
}

func asCustomError(err error) (*util.CustomError, bool) {
	var customErr *util.CustomError
	return customErr, errors.As(err, &customErr)
}

func execute(
	h handlerfunc, w http.ResponseWriter, httpReq *http.Request,
) error {
	req, err := request.NewRequest(httpReq, w)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	resp, err := h(req)
	if err != nil {
		return fmt.Errorf("handler: %w", err)
	}
	resp.Respond(w, httpReq)
	return nil
}
