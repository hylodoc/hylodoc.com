package postbody

import (
	"fmt"
	"os"

	"github.com/knuthic/knuthic/internal/model"
)

type PostBody interface {
	Read(model.EmailMode) ([]byte, error)
}

type postbody struct {
	htmlpath, plaintextpath string
}

func NewPostBody(htmlpath, plaintextpath string) PostBody {
	return &postbody{htmlpath, plaintextpath}
}

func (b *postbody) Read(m model.EmailMode) ([]byte, error) {
	switch m {
	case model.EmailModeHtml:
		return os.ReadFile(b.htmlpath)
	case model.EmailModePlaintext:
		return os.ReadFile(b.plaintextpath)
	default:
		return nil, fmt.Errorf("unknown email mode %q", m)
	}
}
