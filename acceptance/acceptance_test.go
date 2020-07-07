package acceptance

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

var buildDir string

func TestVersion(t *testing.T) {
	var err error
	buildDir, err = ioutil.TempDir("", "lifecycle-acceptance")
	h.AssertNil(t, err)
	defer func() {
		h.AssertNil(t, os.RemoveAll(buildDir))
	}()
	h.BuildLifecycle(t, buildDir, runtime.GOOS)
	spec.Run(t, "acceptance", testVersion, spec.Parallel(), spec.Report(report.Terminal{}))
}

type testCase struct {
	description string
	command     string
	args        []string
}

func testVersion(t *testing.T, when spec.G, it spec.S) {
	when("All", func() {
		when("CNB_PLATFORM_API is set and incompatible", func() {
			for _, phase := range []string{
				"analyzer",
				"builder",
				"detector",
				"exporter",
				"restorer",
				"rebaser",
				"lifecycle",
			} {
				phase := phase
				it(phase+"/should fail with error message and exit code 11", func() {
					cmd := lifecycleCmd(phase)
					cmd.Env = append(os.Environ(), "CNB_PLATFORM_API=0.8")

					_, exitCode, err := h.RunE(cmd)
					h.AssertError(t, err, "the Lifecycle's Platform API version is 0.9 which is incompatible with Platform API version 0.8")
					h.AssertEq(t, exitCode, 11)
				})
			}
		})

		when("version flag is set", func() {
			for _, tc := range []testCase{
				{
					description: "detector: only -version is present",
					command:     "detector",
					args:        []string{"-version"},
				},
				{
					description: "detector: other params are set",
					command:     "detector",
					args:        []string{"-app=/some/dir", "-version"},
				},
				{
					description: "analyzer: only -version is present",
					command:     "analyzer",
					args:        []string{"-version"},
				},
				{
					description: "analyzer: other params are set",
					command:     "analyzer",
					args:        []string{"-daemon", "-version"},
				},
				{
					description: "restorer: only -version is present",
					command:     "restorer",
					args:        []string{"-version"},
				},
				{
					description: "restorer: other params are set",
					command:     "restorer",
					args:        []string{"-cache-dir=/some/dir", "-version"},
				},
				{
					description: "restorer: only -version is present",
					command:     "restorer",
					args:        []string{"-version"},
				},
				{
					description: "restorer: other params are set",
					command:     "restorer",
					args:        []string{"-cache-dir=/some/dir", "-version"},
				},
				{
					description: "builder: only -version is present",
					command:     "builder",
					args:        []string{"-version"},
				},
				{
					description: "builder: other params are set",
					command:     "builder",
					args:        []string{"-app=/some/dir", "-version"},
				},
				{
					description: "exporter: only -version is present",
					command:     "exporter",
					args:        []string{"-version"},
				},
				{
					description: "exporter: other params are set",
					command:     "exporter",
					args:        []string{"-app=/some/dir", "-version"},
				},
				{
					description: "rebaser: only -version is present",
					command:     "rebaser",
					args:        []string{"-version"},
				},
				{
					description: "rebaser: other params are set",
					command:     "rebaser",
					args:        []string{"-daemon", "-version"},
				},
				{
					description: "lifecycle -version",
					command:     "lifecycle",
					args:        []string{"-version"},
				},
			} {
				tc := tc
				when(tc.description, func() {
					it("only prints the version", func() {
						cmd := lifecycleCmd(tc.command, tc.args...)
						output, err := cmd.CombinedOutput()
						if err != nil {
							t.Fatalf("failed to run %v\n OUTPUT: %s\n ERROR: %s\n", cmd.Args, output, err)
						}
						h.AssertStringContains(t, string(output), "some-version+asdf123")
					})
				})
			}
		})
	})
}

func lifecycleCmd(phase string, args ...string) *exec.Cmd {
	return exec.Command(filepath.Join(buildDir, runtime.GOOS, "lifecycle", phase), args...)
}

func buildTestImage(t *testing.T, name, context string) {
	cmd := exec.Command("docker", "build", "-t", name, context)
	h.Run(t, cmd)
}

func removeTestImage(t *testing.T, name string) { // TODO: move to helpers
	t.Helper()
	cmd := exec.Command("docker", "rmi", name)
	h.Run(t, cmd)
}
