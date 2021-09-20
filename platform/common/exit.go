package common

const (
	// lifecycle errors not specific to any phase: 1-99
	CodeFailed = 1 // CodeFailed indicates generic lifecycle error
	// 2: reserved
	CodeInvalidArgs = 3
	// 4: CodeInvalidEnv
	// 5: CodeNotFound
	// 9: CodeFailedUpdate

	// API errors
	CodeIncompatiblePlatformAPI  = 11
	CodeIncompatibleBuildpackAPI = 12
)

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

type LifecycleExitError int
