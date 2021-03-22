package v05

var exitCodes = map[string]int{
	// detect phase errors: 100-199
	"FailedDetect":           100, // FailedDetect indicates that no buildpacks detected
	"FailedDetectWithErrors": 101, // FailedDetectWithErrors indicated that no buildpacks detected and at least one errored
	"DetectError":            102, // DetectError indicates generic detect error

	// analyze phase errors: 200-299
	"AnalyzeError": 202, // AnalyzeError indicates generic analyze error

	// restore phase errors: 300-399
	"RestoreError": 302, // RestoreError indicates generic restore error

	// build phase errors: 400-499
	"FailedBuildWithErrors": 401, // FailedBuildWithErrors indicates buildpack error during /bin/build
	"BuildError":            402, // BuildError indicates generic build error

	// export phase errors: 500-599
	"ExportError": 502, // ExportError indicates generic export error

	// rebase phase errors: 600-699
	"RebaseError": 602, // RebaseError indicates generic rebase error

	// launch phase errors: 700-799
	"LaunchError": 702, // LaunchError indicates generic launch error
}

func (p *Platform) CodeFor(errType string) int {
	return exitCodes[errType]
}
