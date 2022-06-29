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
	switch platformAPI {
	case "0.3", "0.4", "0.5":
		return &LegacyExiter{}
	default:
		return &DefaultExiter{}
	}
}

type DefaultExiter struct{}

var defaultExitCodes = map[LifecycleExitError]int{
	// launch phase errors: 80-89
	LaunchError: 82, // LaunchError indicates generic launch error
}

func (e *DefaultExiter) CodeFor(errType LifecycleExitError) int {
	return codeFor(errType, defaultExitCodes)
}

type LegacyExiter struct{}

var legacyExitCodes = map[LifecycleExitError]int{
	// launch phase errors: 700-799
	LaunchError: 702, // LaunchError indicates generic launch error
}

func (e *LegacyExiter) CodeFor(errType LifecycleExitError) int {
	return codeFor(errType, legacyExitCodes)
}

func codeFor(errType LifecycleExitError, exitCodes map[LifecycleExitError]int) int {
	if code, ok := exitCodes[errType]; ok {
		return code
	}
	return CodeForFailed
}
