package response

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/hylodoc/hylodoc.com/internal/config"
	"github.com/hylodoc/hylodoc.com/internal/util"
)

type tmpl struct {
	names []string
	info  util.PageInfo
}

func NewTemplate(names []string, info util.PageInfo) Response {
	return &tmpl{names, info}
}

func (t *tmpl) Respond(w http.ResponseWriter, _ *http.Request) error {
	var tmp bytes.Buffer
	if err := util.ExecTemplate(
		&tmp, t.names, t.info,
		fmt.Sprintf(
			"%s://%s",
			config.Config.Hylodoc.Protocol,
			config.Config.Hylodoc.RootDomain,
		),
		config.Config.Hylodoc.CDN,
	); err != nil {
		return err
	}
	_, err := w.Write(tmp.Bytes())
	return err
}
