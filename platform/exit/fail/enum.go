package fail // TODO: rename

type ExitCode int

const (
	Failed ExitCode = iota
	InvalidArgs
	IncompatiblePlatformAPI
	IncompatibleBuildpackAPI
	FailedDetect             // generic detect error
	FailedDetectWithErrors   // no buildpacks detected
	DetectError              // no buildpacks detected and at least one errored
	AnalyzeError             // generic analyze error
	RestoreError             // generic restore error
	FailedBuildWithErrors    // buildpack error during /bin/build
	BuildError               // generic build error
	ExportError              // generic export error
	RebaseError              // generic rebase error
	LaunchError              // generic launch error
	FailedGenerateWithErrors // extension error during /bin/generate
	GenerateError            // generic generate error
	ExtendError              // generic extend error
)
