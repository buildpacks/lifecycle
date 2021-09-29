package common

type LifecycleExitError int

const CodeFailed = 1

const (
	FailedDetect           LifecycleExitError = iota
	FailedDetectWithErrors                    // no buildpacks detected
	DetectError                               // no buildpacks detected and at least one errored
	AnalyzeError                              // generic analyze error
	RestoreError                              // generic restore error
	FailedBuildWithErrors                     // buildpack error during /bin/build
	BuildError                                // generic build error
	ExportError                               // generic export error
	RebaseError                               // generic rebase error
	LaunchError                               // generic launch error
)

func CodeFor(errType LifecycleExitError, exitCodes map[LifecycleExitError]int) int {
	if code, ok := exitCodes[errType]; ok {
		return code
	}
	return CodeFailed
}
