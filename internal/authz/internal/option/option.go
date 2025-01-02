package option

import "fmt"

type Option interface {
	Value() bool

	fmt.Stringer
}

type option bool

func New(can bool) Option { return option(can) }

func (opt option) Value() bool { return bool(opt) }
func (opt option) String() string {
	if opt {
		return "yes"
	}
	return "no"
}
