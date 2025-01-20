package response

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/knuthic/knuthic/internal/config"
	"github.com/knuthic/knuthic/internal/util"
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
			config.Config.Knuthic.Protocol,
			config.Config.Knuthic.RootDomain,
		),
		config.Config.Knuthic.CDN,
	); err != nil {
		return err
	}
	_, err := w.Write(tmp.Bytes())
	return err
}
