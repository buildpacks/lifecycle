package common

import "github.com/buildpacks/lifecycle/cmd"

const (
	LaunchError LifecycleExitError = iota // generic launch error
)

type LifecycleExitError int

var exitCodes = map[LifecycleExitError]int{
	// launch phase errors: 80-89
	LaunchError: 82, // LaunchError indicates generic launch error
}

func CodeFor(errType LifecycleExitError) int {
	if code, ok := exitCodes[errType]; ok {
		return code
	}
	return cmd.CodeFailed
}
