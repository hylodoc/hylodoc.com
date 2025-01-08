package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/xr0-org/progstack/internal/app/handler/response"
	"github.com/xr0-org/progstack/internal/assert"
	"github.com/xr0-org/progstack/internal/authz"
	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/session"
	"github.com/xr0-org/progstack/internal/util"
)

func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)

	if errors.Is(err, authz.SubscriptionError) {
		sesh.Println("authz error:", err)
		unauthorised(w, r)
		return
	}

	sesh.Println("internal server error:", err)
	internalServerError(w, r)
}

func internalServerError(w http.ResponseWriter, r *http.Request) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)
	w.WriteHeader(http.StatusInternalServerError)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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

func unauthorised(w http.ResponseWriter, r *http.Request) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)
	w.WriteHeader(http.StatusInternalServerError)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := response.NewTemplate(
		[]string{"401.html"},
		util.PageInfo{
			Data: struct {
				Title    string
				UserInfo *session.UserInfo
			}{
				Title:    "Progstack – Unauthorised",
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

func NotFound(w http.ResponseWriter, r *http.Request) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)
	sesh.Println("404", r.URL)
	w.WriteHeader(http.StatusNotFound)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
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

func NotFoundSubdomain(w http.ResponseWriter, r *http.Request) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)
	sesh.Println("404 (subdomain)", r.Host, r.URL)
	w.WriteHeader(http.StatusNotFound)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := response.NewTemplate(
		[]string{"404_subdomain.html"},
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
				RequestedSubdomain: r.Host,
				StartURL: fmt.Sprintf(
					"%s://%s/register",
					config.Config.Progstack.Protocol,
					config.Config.Progstack.RootDomain,
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

func NotFoundDomain(w http.ResponseWriter, r *http.Request) {
	sesh, ok := r.Context().Value(session.CtxSessionKey).(*session.Session)
	assert.Assert(ok)
	sesh.Println("404 (domain)", r.Host, r.URL)
	w.WriteHeader(http.StatusNotFound)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := response.NewTemplate(
		[]string{"404_domain.html"},
		util.PageInfo{
			Data: struct {
				Title           string
				UserInfo        *session.UserInfo
				Progstack       string
				RequestedDomain string
				DiscordURL      string
				DomainGuideURL  string
			}{
				Title:           "Progstack – Site not found",
				UserInfo:        session.ConvertSessionToUserInfo(sesh),
				Progstack:       config.Config.Progstack.Progstack,
				RequestedDomain: r.Host,
				DomainGuideURL:  config.Config.Progstack.CustomDomainGuideURL,
				DiscordURL:      config.Config.Progstack.DiscordURL,
			},
		},
	).Respond(w, r); err != nil {
		sesh.Println(
			"pathological error:",
			err,
		)
	}
}
