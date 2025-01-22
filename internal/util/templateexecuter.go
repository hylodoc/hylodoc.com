package util

import (
	"fmt"
	"html/template"
	"io"
	"path"
	"strings"
)

const (
	Tmpldir = "web/templates"
)

/* ExecTemplate */

func prependDir(names []string, dir string) []string {
	joined := make([]string, len(names))
	for i := range names {
		joined[i] = path.Join(Tmpldir, dir, names[i])
	}
	return joined
}

/* present on every page */
var pageTemplates []string = []string{
	"base.html", "navbar.html",
}

type PageInfo struct {
	Data       interface{}
	NewUpdates bool
}

func ExecTemplate(
	w io.Writer, names []string, info PageInfo,
	hylodocurl, cdnurl string,
) error {
	tmpl, err := template.New(names[0]).Funcs(
		template.FuncMap{
			"join": strings.Join,
		},
	).ParseFiles(
		append(
			prependDir(names, "pages"),
			prependDir(pageTemplates, "partials")...,
		)...,
	)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}
	if err := tmpl.Execute(w, struct {
		PageInfo
		HylodocURL string
		CDN          string
	}{info, hylodocurl, cdnurl}); err != nil {
		return fmt.Errorf("execute: %w", err)
	}
	return nil
}
