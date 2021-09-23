package common

import "github.com/buildpacks/lifecycle/cmd"

const (
	FailedDetect           LifecycleExitError = iota // generic detect error
	FailedDetectWithErrors                           // no buildpacks detected
	DetectError                                      // no buildpacks detected and at least one errored
	AnalyzeError                                     // generic analyze error
	RestoreError                                     // generic restore error
	FailedBuildWithErrors                            // buildpack error during /bin/build
	BuildError                                       // generic build error
	ExportError                                      // generic export error
	RebaseError                                      // generic rebase error
)

type LifecycleExitError int

func CodeFor(errType LifecycleExitError, exitCodes map[LifecycleExitError]int) int {
	if code, ok := exitCodes[errType]; ok {
		return code
	}
	return cmd.CodeFailed
}
