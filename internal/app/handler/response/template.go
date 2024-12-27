package response

import (
	"log"
	"net/http"
	"text/template"

	"github.com/xr0-org/progstack/internal/util"
)

type tmpl struct {
	names   []string
	info    util.PageInfo
	funcMap template.FuncMap
	logger  *log.Logger
}

func NewTemplate(
	names []string, info util.PageInfo,
	funcMap template.FuncMap, logger *log.Logger,
) Response {
	return &tmpl{names, info, funcMap, logger}
}

func (t *tmpl) Respond(w http.ResponseWriter, _ *http.Request) error {
	return util.ExecTemplate(w, t.names, t.info, t.funcMap, t.logger)
}
