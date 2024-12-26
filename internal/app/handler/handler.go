package handler

import (
	"fmt"
	"net/http"

	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/model"
)

type HandlerFunc func(request.Request) (response.Response, error)

func AsHttp(h HandlerFunc, s *model.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := execute(h, s, w, r); err != nil {
			/* TODO: error page */
			logging.Logger(r).Println("AsHttp error", err)
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
	}
}

func execute(
	h HandlerFunc, s *model.Store,
	w http.ResponseWriter, httpReq *http.Request,
) error {
	req, err := request.NewRequest(httpReq)
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
