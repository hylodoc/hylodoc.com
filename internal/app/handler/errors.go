package handler

import (
	"fmt"
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

func NotFoundSubdomain(
	w http.ResponseWriter, r *http.Request, subdomain string,
) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)
	sesh.Println("404 (subdomain)", subdomain, r.URL)
	w.WriteHeader(http.StatusNotFound)
	if err := response.NewTemplate(
		[]string{"404.html"},
		util.PageInfo{
			Data: struct {
				Title              string
				UserInfo           *session.UserInfo
				Progstack          string
				RequestedSubdomain string
				StartURL           string
			}{
				Title:              "Progstack – Site not found",
				UserInfo:           session.ConvertSessionToUserInfo(sesh),
				Progstack:          config.Config.Progstack.Progstack,
				RequestedSubdomain: subdomain,
				StartURL: fmt.Sprintf(
					"%s://%s/register",
					config.Config.Progstack.Protocol,
					config.Config.Progstack.ServiceName,
				),
			},
		},
	).Respond(w, r); err != nil {
		sesh.Println(
			"pathological error:",
			err,
		)
	}
}
