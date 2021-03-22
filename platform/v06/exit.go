package v06

var exitCodes = map[string]int{
	// detect phase errors: 20-29
	"FailedDetect":           20, // FailedDetect indicates that no buildpacks detected
	"FailedDetectWithErrors": 21, // FailedDetectWithErrors indicated that no buildpacks detected and at least one errored
	"DetectError":            22, // DetectError indicates generic detect error

	// analyze phase errors: 30-39
	"AnalyzeError": 32, // AnalyzeError indicates generic analyze error

	// restore phase errors: 40-49
	"RestoreError": 42, // RestoreError indicates generic restore error

	// build phase errors: 50-59
	"FailedBuildWithErrors": 51, // FailedBuildWithErrors indicates buildpack error during /bin/build
	"BuildError":            52, // BuildError indicates generic build error

	// export phase errors: 60-69
	"ExportError": 62, // ExportError indicates generic export error

	// rebase phase errors: 70-79
	"RebaseError": 72, // RebaseError indicates generic rebase error

	// launch phase errors: 80-89
	"LaunchError": 82, // LaunchError indicates generic launch error
}

func (p *Platform) CodeFor(errType string) int {
	return exitCodes[errType]
}
