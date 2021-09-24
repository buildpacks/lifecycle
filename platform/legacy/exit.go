package legacy

import (
	"github.com/buildpacks/lifecycle/platform/common"
)

var exitCodes = map[common.LifecycleExitError]int{
	// detect phase errors: 100-199
	common.FailedDetect:           100, // FailedDetect indicates that no buildpacks detected
	common.FailedDetectWithErrors: 101, // FailedDetectWithErrors indicated that no buildpacks detected and at least one errored
	common.DetectError:            102, // DetectError indicates generic detect error

	// analyze phase errors: 200-299
	common.AnalyzeError: 202, // AnalyzeError indicates generic analyze error

	// restore phase errors: 300-399
	common.RestoreError: 302, // RestoreError indicates generic restore error

	// build phase errors: 400-499
	common.FailedBuildWithErrors: 401, // FailedBuildWithErrors indicates buildpack error during /bin/build
	common.BuildError:            402, // BuildError indicates generic build error

	// export phase errors: 500-599
	common.ExportError: 502, // ExportError indicates generic export error

	// rebase phase errors: 600-699
	common.RebaseError: 602, // RebaseError indicates generic rebase error

	// launch phase errors: 700-799
	common.LaunchError: 702, // LaunchError indicates generic launch error
}

func (p *legacyPlatform) CodeFor(errType common.LifecycleExitError) int {
	return common.CodeFor(errType, exitCodes)
}
