package handler

import (
	"net/http"

	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

func internalServerError(w http.ResponseWriter, r *http.Request) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)
	w.WriteHeader(http.StatusInternalServerError)
	if err := response.NewTemplate(
		[]string{"500.html"},
		util.PageInfo{
			Data: struct {
				Title        string
				UserInfo     *session.UserInfo
				DiscordURL   string
				OpenIssueURL string
			}{
				Title:        "Progstack – Internal server error",
				UserInfo:     session.ConvertSessionToUserInfo(sesh),
				DiscordURL:   config.Config.Progstack.DiscordURL,
				OpenIssueURL: config.Config.Progstack.OpenIssueURL,
			},
		},
	).Respond(w, r); err != nil {
		sesh.Println(
			"pathological error:",
			err,
		)
	}
}

func NotFound(w http.ResponseWriter, r *http.Request) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)
	sesh.Println("404", r.URL)
	w.WriteHeader(http.StatusNotFound)
	if err := response.NewTemplate(
		[]string{"404.html"},
		util.PageInfo{
			Data: struct {
				Title    string
				UserInfo *session.UserInfo
			}{
				Title:    "Progstack – Page not found",
				UserInfo: session.ConvertSessionToUserInfo(sesh),
			},
		},
	).Respond(w, r); err != nil {
		sesh.Println(
			"pathological error:",
			err,
		)
	}
}
