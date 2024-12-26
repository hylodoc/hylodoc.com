package util

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path"
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
	w http.ResponseWriter, names []string, info PageInfo,
	funcMap template.FuncMap, logger *log.Logger,
) {
	if err := tryExecTemplate(w, names, info, funcMap, logger); err != nil {
		http.Error(
			w, "error loading page", http.StatusInternalServerError,
		)
	}
}

func tryExecTemplate(
	w http.ResponseWriter, names []string, info PageInfo,
	funcMap template.FuncMap, logger *log.Logger,
) error {
	tmpl, err := template.New(names[0]).Funcs(funcMap).ParseFiles(
		append(
			prependDir(names, "pages"),
			prependDir(pageTemplates, "partials")...,
		)...,
	)
	if err != nil {
		return fmt.Errorf("load: %w", err)
	}
	if err := tmpl.Execute(w, info); err != nil {
		return fmt.Errorf("execute: %w", err)
	}
	return nil
}
