package exit

import (
	"fmt"
	"strings"
)

type Error struct {
	Err    error
	Code   int
	Action []string
}

func (e *Error) Error() string {
	message := "failed to " + strings.Join(e.Action, " ")
	if e.Err == nil {
		return message
	}
	return fmt.Sprintf("%s: %s", message, e.Err)
}

func ErrorFromCode(code int, action ...string) *Error {
	return ErrorFromErrAndCode(nil, code, action...)
}

func ErrorFromErr(err error, action ...string) *Error {
	code := CodeForFailed
	if err, ok := err.(*Error); ok {
		code = err.Code
	}
	return ErrorFromErrAndCode(err, code, action...)
}

func ErrorFromErrAndCode(err error, code int, action ...string) *Error {
	return &Error{Err: err, Code: code, Action: action}
}
