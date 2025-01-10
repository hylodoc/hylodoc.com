package valueset

import "github.com/xr0-org/progstack/internal/assert"

type Set interface {
	HasRepository() bool
	Repository() int64
	Subdomain() string
	Theme() string
	Branch() string
}

type v struct {
	r       int64
	hasrepo bool
	s, t, b string
}

func NewEmpty() Set { return &v{} }

func New(repository int64, subdomain, theme, branch string) Set {
	return &v{repository, true, subdomain, theme, branch}
}

func (v *v) HasRepository() bool { return v.hasrepo }

func (v *v) Repository() int64 {
	assert.Assert(v.HasRepository())
	return v.r
}

func (v *v) Subdomain() string { return v.s }
func (v *v) Theme() string     { return v.t }
func (v *v) Branch() string    { return v.b }
