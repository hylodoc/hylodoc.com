package authz

import "errors"

var SubscriptionError = errors.New("subscription error")

type suberr struct {
	err error
}

func newSubErr(err error) *suberr { return &suberr{err} }

func (err *suberr) Error() string        { return err.Error() }
func (err *suberr) Is(target error) bool { return target == SubscriptionError }
