package exit

import "github.com/buildpacks/lifecycle/platform/exit/fail"

const (
	CodeForFailed                   = 1
	CodeForInvalidArgs              = 3
	CodeForIncompatiblePlatformAPI  = 11
	CodeForIncompatibleBuildpackAPI = 12
)

type Exiter interface {
	CodeFor(errType fail.ExitCode) int
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

var defaultExitCodes = map[fail.ExitCode]int{
	// generic errors
	fail.Failed:                   1,
	fail.InvalidArgs:              3,
	fail.IncompatiblePlatformAPI:  11,
	fail.IncompatibleBuildpackAPI: 12,

	// detect phase errors: 20-29
	fail.FailedDetect:           20, // FailedDetect indicates that no buildpacks detected
	fail.FailedDetectWithErrors: 21, // FailedDetectWithErrors indicated that no buildpacks detected and at least one errored
	fail.DetectError:            22, // DetectError indicates generic detect error

	// analyze phase errors: 30-39
	fail.AnalyzeError: 32, // AnalyzeError indicates generic analyze error

	// restore phase errors: 40-49
	fail.RestoreError: 42, // RestoreError indicates generic restore error

	// build phase errors: 50-59
	fail.FailedBuildWithErrors: 51, // FailedBuildWithErrors indicates buildpack error during /bin/build
	fail.BuildError:            52, // BuildError indicates generic build error

	// export phase errors: 60-69
	fail.ExportError: 62, // ExportError indicates generic export error

	// rebase phase errors: 70-79
	fail.RebaseError: 72, // RebaseError indicates generic rebase error

	// launch phase errors: 80-89
	fail.LaunchError: 82, // LaunchError indicates generic launch error

	// generate phase errors: 90-99
	fail.FailedGenerateWithErrors: 91, // FailedGenerateWithErrors indicates extension error during /bin/generate
	fail.GenerateError:            92, // GenerateError indicates generic generate error

	// extend phase errors: 100-109
	fail.ExtendError: 102, // ExtendError indicates generic extend error
}

func (e *DefaultExiter) CodeFor(errType fail.ExitCode) int {
	return codeFor(errType, defaultExitCodes)
}

type LegacyExiter struct{}

var legacyExitCodes = map[fail.ExitCode]int{
	// generic errors
	fail.Failed:                   1,
	fail.InvalidArgs:              3,
	fail.IncompatiblePlatformAPI:  11,
	fail.IncompatibleBuildpackAPI: 12,

	// detect phase errors: 100-199
	fail.FailedDetect:           100, // FailedDetect indicates that no buildpacks detected
	fail.FailedDetectWithErrors: 101, // FailedDetectWithErrors indicated that no buildpacks detected and at least one errored
	fail.DetectError:            102, // DetectError indicates generic detect error

	// analyze phase errors: 200-299
	fail.AnalyzeError: 202, // AnalyzeError indicates generic analyze error

	// restore phase errors: 300-399
	fail.RestoreError: 302, // RestoreError indicates generic restore error

	// build phase errors: 400-499
	fail.FailedBuildWithErrors: 401, // FailedBuildWithErrors indicates buildpack error during /bin/build
	fail.BuildError:            402, // BuildError indicates generic build error

	// export phase errors: 500-599
	fail.ExportError: 502, // ExportError indicates generic export error

	// rebase phase errors: 600-699
	fail.RebaseError: 602, // RebaseError indicates generic rebase error

	// launch phase errors: 700-799
	fail.LaunchError: 702, // LaunchError indicates generic launch error

	// generate phase is unsupported on older platforms and shouldn't be reached
	fail.FailedGenerateWithErrors: CodeForFailed,
	fail.GenerateError:            CodeForFailed,

	// extend phase is unsupported on older platforms and shouldn't be reached
	fail.ExtendError: CodeForFailed,
}

func (e *LegacyExiter) CodeFor(errType fail.ExitCode) int {
	return codeFor(errType, legacyExitCodes)
}

func codeFor(errType fail.ExitCode, exitCodes map[fail.ExitCode]int) int {
	if code, ok := exitCodes[errType]; ok {
		return code
	}
	return CodeForFailed
}
