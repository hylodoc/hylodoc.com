package emailtemplate

import (
	"fmt"

	"github.com/hylodoc/hylodoc.com/internal/model"
)

const (
	newpostPlaintext = `{{ .PostBody }}

---

View this post at {{ .PostLink }}.

If you would like to stop receiving these emails, click
	{{ .UnsubscribeLink }}
to unsubscribe.`

	newpostHtml = `
<!DOCTYPE HTML>
<html>
<body>
{{ .PostBody }}

<hr>
<p>
	View this post <a href="{{ .PostLink }}">here</a>.
</p>

<p>
	If you would like to stop receiving these emails, click
		<a href="{{ .UnsubscribeLink }}">here</a>
	to unsubscribe.
</p>
</body>
</html>`
)

type newpost struct {
	PostLink, PostBody, UnsubscribeLink string
}

func NewPost(postlink, postbody, unsublink string) Template {
	return &newpost{postlink, postbody, unsublink}
}

func (t *newpost) Render(mode model.EmailMode) (string, error) {
	switch mode {
	case model.EmailModePlaintext:
		return exectmpl(newpostPlaintext, t)
	case model.EmailModeHtml:
		return exectmpl(newpostHtml, t)
	default:
		return "", fmt.Errorf("unknown email mode: %q", mode)
	}
}
