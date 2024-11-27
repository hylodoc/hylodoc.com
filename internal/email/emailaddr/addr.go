package emailaddr

import "fmt"

type EmailAddr interface {
	Addr() string
}

type addr struct {
	name, email string
}

func NewAddr(email string) EmailAddr            { return &addr{"", email} }
func NewNamedAddr(name, email string) EmailAddr { return &addr{name, email} }

func (a *addr) Addr() string {
	if len(a.name) > 0 {
		return fmt.Sprintf("%s <%s>", a.name, a.email)
	}
	return a.email
}
