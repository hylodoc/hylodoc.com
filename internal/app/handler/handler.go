package handler

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/xr0-org/progstack/internal/app/handler/request"
	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/authz"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/logging"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

func Handle(r *mux.Router, pattern string, h handlerfunc) *mux.Route {
	return r.HandleFunc(
		pattern,
		func(w http.ResponseWriter, r *http.Request) {
			sesh, ok := r.Context().Value(
				session.CtxSessionKey,
			).(*session.Session)
			assert.Printf(ok, "no session")
			if err := execute(h, sesh, w, r); err != nil {
				logger := logging.Logger(r)
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
				w.WriteHeader(http.StatusInternalServerError)
				if err := response.NewTemplate(
					[]string{"internal_server_error.html"},
					util.PageInfo{
						Data: struct {
							Title        string
							UserInfo     *session.UserInfo
							DiscordURL   string
							OpenIssueURL string
						}{
							Title:        "Progstack – Internal Server Error",
							UserInfo:     session.ConvertSessionToUserInfo(sesh),
							DiscordURL:   config.Config.Progstack.DiscordURL,
							OpenIssueURL: config.Config.Progstack.OpenIssueURL,
						},
					},
					template.FuncMap{},
					logger,
				).Respond(w, r); err != nil {
					logger.Println(
						"pathological error:",
						err,
					)
				}
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
	h handlerfunc, sesh *session.Session,
	w http.ResponseWriter, r *http.Request,
) error {
	resp, err := h(request.NewRequest(r, w, sesh))
	if err != nil {
		return fmt.Errorf("handler: %w", err)
	}
	if err := resp.Respond(w, r); err != nil {
		return fmt.Errorf("respond: %w", err)
	}
	return nil
}
