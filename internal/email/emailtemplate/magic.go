package emailtemplate

import (
	"fmt"

	"github.com/xr0-org/progstack/internal/model"
)

const (
	registerlinkPlaintext = `Click here to register: {{ .Link }}.`
	registerlinkHtml      = `
<!DOCTYPE HTML>
<html>
<body>
<p>
	Click <a href="{{ .Link }}">here</a> to register.
</p>
</body>
</html>`

	loginlinkPlaintext = `Click here to log in: {{ .Link }}.`
	loginlinkHtml      = `
<!DOCTYPE HTML>
<html>
<body>
<p>
	Click <a href="{{ .Link }}">here</a> to log in.
</p>
</body>
</html>`
)

type registerlink struct {
	Link string
}

func NewRegisterLink(link string) Template {
	return &registerlink{link}
}

func (t *registerlink) Render(mode model.EmailMode) (string, error) {
	switch mode {
	case model.EmailModePlaintext:
		return exectmpl(registerlinkPlaintext, t)
	case model.EmailModeHtml:
		return exectmpl(registerlinkHtml, t)
	default:
		return "", fmt.Errorf("unknown email mode: %q", mode)
	}
}

type loginlink struct {
	Link string
}

func NewLoginLink(link string) Template {
	return &loginlink{link}
}

func (t *loginlink) Render(mode model.EmailMode) (string, error) {
	switch mode {
	case model.EmailModePlaintext:
		return exectmpl(loginlinkPlaintext, t)
	case model.EmailModeHtml:
		return exectmpl(loginlinkHtml, t)
	default:
		return "", fmt.Errorf("unknown email mode: %q", mode)
	}
}
