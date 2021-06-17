package pre06

import (
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/platform"
)

var exitCodes = map[platform.LifecycleExitError]int{
	// detect phase errors: 100-199
	platform.FailedDetect:           100, // FailedDetect indicates that no buildpacks detected
	platform.FailedDetectWithErrors: 101, // FailedDetectWithErrors indicated that no buildpacks detected and at least one errored
	platform.DetectError:            102, // DetectError indicates generic detect error

	// analyze phase errors: 200-299
	platform.AnalyzeError: 202, // AnalyzeError indicates generic analyze error

	// restore phase errors: 300-399
	platform.RestoreError: 302, // RestoreError indicates generic restore error

	// build phase errors: 400-499
	platform.FailedBuildWithErrors: 401, // FailedBuildWithErrors indicates buildpack error during /bin/build
	platform.BuildError:            402, // BuildError indicates generic build error

	// export phase errors: 500-599
	platform.ExportError: 502, // ExportError indicates generic export error

	// rebase phase errors: 600-699
	platform.RebaseError: 602, // RebaseError indicates generic rebase error

	// launch phase errors: 700-799
	platform.LaunchError: 702, // LaunchError indicates generic launch error
}

func (p *Platform) CodeFor(errType platform.LifecycleExitError) int {
	if code, ok := exitCodes[errType]; ok {
		return code
	}
	return cmd.CodeFailed
}
