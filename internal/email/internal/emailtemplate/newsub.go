package emailtemplate

import (
	"fmt"

	"github.com/hylodoc/hylodoc.com/internal/model"
)

const (
	newsubPlaintext = `Thank you for subscribing to {{ .SiteTitle }}.

From now on you'll receive new posts here in your inbox.

Cheers,
{{ .SiteTitle }}

---

If you didn't sign up for this, click
	{{ .UnsubscribeLink }}
to unsubscribe.`

	newsubHtml = `
<!DOCTYPE HTML>
<html>
<body>
<p>
	Thank you for subscribing to {{ .SiteTitle }}.
</p>

<p>
	From now on you'll receive new posts here in your inbox.
</p>

<p>
	Cheers,
	<br>
	{{ .SiteTitle }}
</p>

<hr>

<p>
	If you didn't sign up for this, click
		<a href="{{ .UnsubscribeLink }}">here</a>
	to unsubscribe.
</p>
</body>
</html>`
)

type newsub struct {
	SiteTitle       string
	UnsubscribeLink string
}

func NewSubscriber(sitetitle, unsublink string) Template {
	return &newsub{sitetitle, unsublink}
}

func (t *newsub) Render(mode model.EmailMode) (string, error) {
	switch mode {
	case model.EmailModePlaintext:
		return exectmpl(newsubPlaintext, t)
	case model.EmailModeHtml:
		return exectmpl(newsubHtml, t)
	default:
		return "", fmt.Errorf("unknown email mode: %q", mode)
	}
}
