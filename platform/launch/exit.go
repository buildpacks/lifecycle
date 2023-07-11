package launch

type LifecycleExitError int

const CodeForFailed = 1

const (
	LaunchError LifecycleExitError = iota
)

type Exiter interface {
	CodeFor(errType LifecycleExitError) int
}

func NewExiter(platformAPI string) Exiter {
	return &DefaultExiter{}
}

type DefaultExiter struct{}

var defaultExitCodes = map[LifecycleExitError]int{
	// launch phase errors: 80-89
	LaunchError: 82, // LaunchError indicates generic launch error
}

func (e *DefaultExiter) CodeFor(errType LifecycleExitError) int {
	return codeFor(errType, defaultExitCodes)
}

func codeFor(errType LifecycleExitError, exitCodes map[LifecycleExitError]int) int {
	if code, ok := exitCodes[errType]; ok {
		return code
	}
	return CodeForFailed
}
