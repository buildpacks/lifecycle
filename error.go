package lifecycle

type ErrorType string

const ErrTypeBuildpack ErrorType = "ERR_BUILDPACK"
const ErrTypeFailedDetection ErrorType = "ERR_FAILED_DETECTION"

type Error struct {
	Errors []error
	Type   ErrorType
}

func (le *Error) Error() string {
	if le.Cause() != nil {
		return le.Cause().Error()
	}
	return ""
}

func (le *Error) Cause() error {
	switch len(le.Errors) {
	case 0:
		return nil
	default:
		return le.Errors[0]
	}
}

func NewLifecycleError(cause error, errType ErrorType) *Error {
	return &Error{Errors: []error{cause}, Type: errType}
}
