package launch

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestPrepareWorkingDirectory(t *testing.T) {
	spec.Run(t, "Process", testProcessInternal, spec.Report(report.Terminal{}))
}

func testProcessInternal(t *testing.T, when spec.G, it spec.S) {
	when("getProcessWorkingDirectory", func() {
		const (
			appDir           = "/app"
			workingDirectory = "/working-directory"
		)

		it("defaults the working directory to the app dir", func() {
			process := Process{}
			actualWorkingDirectory := getProcessWorkingDirectory(process, appDir)
			h.AssertEq(t, actualWorkingDirectory, appDir)
		})

		it("respects a configured working directory", func() {
			process := Process{WorkingDirectory: workingDirectory}
			actualWorkingDirectory := getProcessWorkingDirectory(process, appDir)
			h.AssertEq(t, actualWorkingDirectory, workingDirectory)
		})
	})
}
