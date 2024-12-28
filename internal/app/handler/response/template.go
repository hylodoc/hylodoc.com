package response

import (
	"fmt"
	"net/http"

	"github.com/xr0-org/progstack/internal/config"
	"github.com/xr0-org/progstack/internal/util"
)

type tmpl struct {
	names []string
	info  util.PageInfo
}

func NewTemplate(names []string, info util.PageInfo) Response {
	return &tmpl{names, info}
}

func (t *tmpl) Respond(w http.ResponseWriter, _ *http.Request) error {
	return util.ExecTemplate(
		w, t.names, t.info,
		fmt.Sprintf(
			"%s://%s",
			config.Config.Progstack.Protocol,
			config.Config.Progstack.RootDomain,
		),
		config.Config.Progstack.CDN,
	)
}
