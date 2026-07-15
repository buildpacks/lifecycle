package launch

import (
	"testing"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestPrepareWorkingDirectory(t *testing.T) {
	t.Run("getProcessWorkingDirectory", func(t *testing.T) {
		const (
			appDir           = "/app"
			workingDirectory = "/working-directory"
		)

		t.Run("defaults the working directory to the app dir", func(t *testing.T) {
			process := Process{}
			actualWorkingDirectory := getProcessWorkingDirectory(process, appDir)
			h.AssertEq(t, actualWorkingDirectory, appDir)
		})

		t.Run("respects a configured working directory", func(t *testing.T) {
			process := Process{WorkingDirectory: workingDirectory}
			actualWorkingDirectory := getProcessWorkingDirectory(process, appDir)
			h.AssertEq(t, actualWorkingDirectory, workingDirectory)
		})
	})
}
