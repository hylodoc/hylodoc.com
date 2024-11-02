package util

import (
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

func ExecTemplate(w http.ResponseWriter, names []string, info PageInfo, funcMap template.FuncMap, logger *log.Logger) {
	tmpl, err := template.New(names[0]).Funcs(funcMap).ParseFiles(
		append(
			prependDir(names, "pages"),
			prependDir(pageTemplates, "partials")...,
		)...,
	)
	if err != nil {
		logger.Println("cannot load template", err)
		http.Error(w, "error loading page", http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, info); err != nil {
		logger.Println("cannot execute template", err)
		http.Error(w, "error loading page", http.StatusInternalServerError)
		return
	}
}
