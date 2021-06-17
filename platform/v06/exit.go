package v06

import (
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/platform"
)

var exitCodes = map[platform.LifecycleExitError]int{
	// detect phase errors: 20-29
	platform.FailedDetect:           20, // FailedDetect indicates that no buildpacks detected
	platform.FailedDetectWithErrors: 21, // FailedDetectWithErrors indicated that no buildpacks detected and at least one errored
	platform.DetectError:            22, // DetectError indicates generic detect error

	// analyze phase errors: 30-39
	platform.AnalyzeError: 32, // AnalyzeError indicates generic analyze error

	// restore phase errors: 40-49
	platform.RestoreError: 42, // RestoreError indicates generic restore error

	// build phase errors: 50-59
	platform.FailedBuildWithErrors: 51, // FailedBuildWithErrors indicates buildpack error during /bin/build
	platform.BuildError:            52, // BuildError indicates generic build error

	// export phase errors: 60-69
	platform.ExportError: 62, // ExportError indicates generic export error

	// rebase phase errors: 70-79
	platform.RebaseError: 72, // RebaseError indicates generic rebase error

	// launch phase errors: 80-89
	platform.LaunchError: 82, // LaunchError indicates generic launch error
}

func (p *Platform) CodeFor(errType platform.LifecycleExitError) int {
	if code, ok := exitCodes[errType]; ok {
		return code
	}
	return cmd.CodeFailed
}
