package errorset

type Set interface {
	Repository() string
	Subdomain() string
	Theme() string
	Branch() string
}

type e struct{ r, s, t, b string }

func NewEmpty() Set                { return &e{} }
func NewRepository(msg string) Set { return &e{r: msg} }
func NewSubdomain(msg string) Set  { return &e{s: msg} }
func NewTheme(msg string) Set      { return &e{t: msg} }
func NewBranch(msg string) Set     { return &e{b: msg} }

func (e *e) Repository() string { return e.r }
func (e *e) Subdomain() string  { return e.s }
func (e *e) Theme() string      { return e.t }
func (e *e) Branch() string     { return e.b }
