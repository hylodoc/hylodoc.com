package authz

import "errors"

var SubscriptionError = errors.New("subscription error")

type suberr struct {
	err error
}

func newSubErr(err error) *suberr { return &suberr{err} }

func (se *suberr) Error() string        { return se.err.Error() }
func (se *suberr) Is(target error) bool { return target == SubscriptionError }
