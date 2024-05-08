package cache

import (
	"errors"
)

var errCacheCommitted = errors.New("cache cannot be modified after commit")

type ReadErr struct {
	msg string
}

func NewReadErr(msg string) ReadErr {
	return ReadErr{msg: msg}
}

func (e ReadErr) Error() string {
	return e.msg
}

func IsReadErr(err error) (bool, *ReadErr) {
	var e ReadErr
	isReadErr := errors.As(err, &e)
	return isReadErr, &e
}
