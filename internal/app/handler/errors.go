package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/hylodoc/hylodoc.com/internal/app/handler/response"
	"github.com/hylodoc/hylodoc.com/internal/assert"
	"github.com/hylodoc/hylodoc.com/internal/authz"
	"github.com/hylodoc/hylodoc.com/internal/config"
	"github.com/hylodoc/hylodoc.com/internal/session"
	"github.com/hylodoc/hylodoc.com/internal/util"
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
	if err := response.NewTemplate(
		[]string{"500.html"},
		util.PageInfo{
			Data: struct {
				Title        string
				UserInfo     *session.UserInfo
				DiscordURL   string
				OpenIssueURL string
			}{
				Title:        "Hylodoc – Internal server error",
				UserInfo:     session.ConvertSessionToUserInfoError(sesh),
				DiscordURL:   config.Config.Hylodoc.DiscordURL,
				OpenIssueURL: config.Config.Hylodoc.OpenIssueURL,
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
	if err := response.NewTemplate(
		[]string{"401.html"},
		util.PageInfo{
			Data: struct {
				Title    string
				UserInfo *session.UserInfo
			}{
				Title:    "Hylodoc – Unauthorised",
				UserInfo: session.ConvertSessionToUserInfoError(sesh),
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
				Title:    "Hylodoc – Page not found",
				UserInfo: session.ConvertSessionToUserInfoError(sesh),
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
	sesh.Println("userinfo", session.ConvertSessionToUserInfoError(sesh))
	if err := response.NewTemplate(
		[]string{"404_subdomain.html"},
		util.PageInfo{
			Data: struct {
				Title              string
				UserInfo           *session.UserInfo
				Hylodoc          string
				RequestedSubdomain string
				StartURL           string
			}{
				Title:              "Hylodoc – Site not found",
				UserInfo:           session.ConvertSessionToUserInfoError(sesh),
				Hylodoc:          config.Config.Hylodoc.Hylodoc,
				RequestedSubdomain: r.Host,
				StartURL: fmt.Sprintf(
					"%s://%s",
					config.Config.Hylodoc.Protocol,
					config.Config.Hylodoc.RootDomain,
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
	if err := response.NewTemplate(
		[]string{"404_domain.html"},
		util.PageInfo{
			Data: struct {
				Title           string
				UserInfo        *session.UserInfo
				Hylodoc       string
				RequestedDomain string
				DiscordURL      string
				DomainGuideURL  string
			}{
				Title:           "Hylodoc – Site not found",
				UserInfo:        session.ConvertSessionToUserInfoError(sesh),
				Hylodoc:       config.Config.Hylodoc.Hylodoc,
				RequestedDomain: r.Host,
				DomainGuideURL:  config.Config.Hylodoc.CustomDomainGuideURL,
				DiscordURL:      config.Config.Hylodoc.DiscordURL,
			},
		},
	).Respond(w, r); err != nil {
		sesh.Println(
			"pathological error:",
			err,
		)
	}
}
