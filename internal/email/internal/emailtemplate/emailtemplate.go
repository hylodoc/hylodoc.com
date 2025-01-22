package emailtemplate

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/hylodoc/hylodoc.com/internal/model"
)

type Template interface {
	Render(model.EmailMode) (string, error)
}

func exectmpl(rawtmpl string, data interface{}) (string, error) {
	tmpl, err := template.New("tmpl").Parse(rawtmpl)
	if err != nil {
		return "", fmt.Errorf("cannot parse: %w", err)
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, data); err != nil {
		return "", fmt.Errorf("cannot execute: %w", err)
	}
	return b.String(), nil
}
