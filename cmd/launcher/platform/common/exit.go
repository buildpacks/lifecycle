package common

import "github.com/buildpacks/lifecycle/cmd"

const (
	LaunchError LifecycleExitError = iota // generic launch error
)

type LifecycleExitError int

func CodeFor(errType LifecycleExitError, exitCodes map[LifecycleExitError]int) int {
	if code, ok := exitCodes[errType]; ok {
		return code
	}
	return cmd.CodeFailed
}
