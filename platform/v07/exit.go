package v07

import (
	"github.com/buildpacks/lifecycle/platform/common"
)

var exitCodes = map[common.LifecycleExitError]int{
	// detect phase errors: 20-29
	common.FailedDetect:           20, // FailedDetect indicates that no buildpacks detected
	common.FailedDetectWithErrors: 21, // FailedDetectWithErrors indicated that no buildpacks detected and at least one errored
	common.DetectError:            22, // DetectError indicates generic detect error

	// analyze phase errors: 30-39
	common.AnalyzeError: 32, // AnalyzeError indicates generic analyze error

	// restore phase errors: 40-49
	common.RestoreError: 42, // RestoreError indicates generic restore error

	// build phase errors: 50-59
	common.FailedBuildWithErrors: 51, // FailedBuildWithErrors indicates buildpack error during /bin/build
	common.BuildError:            52, // BuildError indicates generic build error

	// export phase errors: 60-69
	common.ExportError: 62, // ExportError indicates generic export error

	// rebase phase errors: 70-79
	common.RebaseError: 72, // RebaseError indicates generic rebase error

	// launch phase errors: 80-89
	common.LaunchError: 82, // LaunchError indicates generic launch error
}

func (p *v07Platform) CodeFor(errType common.LifecycleExitError) int {
	return common.CodeFor(errType, exitCodes)
}
