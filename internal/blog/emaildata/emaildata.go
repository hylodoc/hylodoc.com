package emaildata

import (
	"html/template"

	"github.com/xr0-org/progstack/internal/assert"
)

type EmailData interface {
	Sent() bool
	Opens() int
	SendURL() template.URL
}

func NewSent(opens int) EmailData          { return sentdata(opens) }
func NewUnsent(url template.URL) EmailData { return unsentdata(url) }

type sentdata int

func (s sentdata) Sent() bool            { return true }
func (s sentdata) Opens() int            { return int(s) }
func (s sentdata) SendURL() template.URL { assert.Assert(false); return "" }

type unsentdata template.URL

func (u unsentdata) Sent() bool            { return false }
func (u unsentdata) Opens() int            { assert.Assert(false); return -1 }
func (u unsentdata) SendURL() template.URL { return template.URL(u) }
