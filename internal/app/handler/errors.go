package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/knuthic/knuthic/internal/app/handler/response"
	"github.com/knuthic/knuthic/internal/assert"
	"github.com/knuthic/knuthic/internal/authz"
	"github.com/knuthic/knuthic/internal/config"
	"github.com/knuthic/knuthic/internal/session"
	"github.com/knuthic/knuthic/internal/util"
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
				Title:        "Knuthic – Internal server error",
				UserInfo:     session.ConvertSessionToUserInfoError(sesh),
				DiscordURL:   config.Config.Knuthic.DiscordURL,
				OpenIssueURL: config.Config.Knuthic.OpenIssueURL,
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
				Title:    "Knuthic – Unauthorised",
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
				Title:    "Knuthic – Page not found",
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
				Knuthic          string
				RequestedSubdomain string
				StartURL           string
			}{
				Title:              "Knuthic – Site not found",
				UserInfo:           session.ConvertSessionToUserInfoError(sesh),
				Knuthic:          config.Config.Knuthic.Knuthic,
				RequestedSubdomain: r.Host,
				StartURL: fmt.Sprintf(
					"%s://%s",
					config.Config.Knuthic.Protocol,
					config.Config.Knuthic.RootDomain,
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
				Knuthic       string
				RequestedDomain string
				DiscordURL      string
				DomainGuideURL  string
			}{
				Title:           "Knuthic – Site not found",
				UserInfo:        session.ConvertSessionToUserInfoError(sesh),
				Knuthic:       config.Config.Knuthic.Knuthic,
				RequestedDomain: r.Host,
				DomainGuideURL:  config.Config.Knuthic.CustomDomainGuideURL,
				DiscordURL:      config.Config.Knuthic.DiscordURL,
			},
		},
	).Respond(w, r); err != nil {
		sesh.Println(
			"pathological error:",
			err,
		)
	}
}
