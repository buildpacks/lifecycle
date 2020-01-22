// +build acceptance

package acceptance

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

type lifecycleCmd func(binary string, args ...string) *exec.Cmd

func TestAcceptance(t *testing.T) {
	buildDir, err := ioutil.TempDir("", "lifecycle-acceptance")
	h.AssertNil(t, err)
	defer h.AssertNil(t, os.RemoveAll(buildDir))
	buildBinaries(t, buildDir)

	spec.Run(t, "acceptance", func(t *testing.T, when spec.G, it spec.S) {
		testAcceptance(t, when, it, func(binary string, args ...string) *exec.Cmd {
			return exec.Command(filepath.Join(buildDir, "lifecycle", binary), args...)
		})
	}, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testAcceptance(t *testing.T, when spec.G, it spec.S, lifecycleCmd lifecycleCmd) {
	var tmpDir string

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "acceptance-*")
		h.AssertNil(t, err)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("All", func() {
		when("CNB_PLATFORM_API", func() {
			tmpWorkingDir := func(sourceDir string) string {
				d, err := ioutil.TempDir(tmpDir, "")
				h.AssertNil(t, err)
				h.RecursiveCopy(t, sourceDir, d)
				return d
			}

			when("is not set", func() {
				it("should complete successfully", func() {
					cmd := lifecycleCmd("analyzer", "some/image")
					cmd.Dir = tmpWorkingDir(filepath.Join("testdata", "analyzer"))

					h.Run(t, cmd)
				})
			})

			when("is set and compatible", func() {
				it("should complete successfully", func() {
					cmd := lifecycleCmd("analyzer", "some/image")
					cmd.Dir = tmpWorkingDir(filepath.Join("testdata", "analyzer"))
					cmd.Env = append(os.Environ(), "CNB_PLATFORM_API=0.2")

					h.Run(t, cmd)
				})
			})

			when("is set and incompatible", func() {
				for _, binary := range []string{
					"analyzer",
					"builder",
					"detector",
					"exporter",
					"restorer",
				} {
					binary := binary
					it(binary+"/should fail with error message and exit code 11", func() {
						cmd := lifecycleCmd(binary)
						cmd.Env = append(os.Environ(), "CNB_PLATFORM_API=99.99")

						_, exitCode, err := h.RunE(cmd)
						h.AssertError(t, err, "the Lifecycle's Platform API version is 0.2 which is incompatible with Platform API version 99.99")
						h.AssertEq(t, exitCode, 11)
					})
				}
			})
		})

		when("version flag is set", func() {
			for _, data := range [][]string{
				{"analyzer: only -version is present", "analyzer -version"},
				{"analyzer: other params are set", "analyzer -daemon -version some/image"},

				{"builder: only -version is present", "builder -version"},
				{"builder: other params are set", "builder -app=/some/dir -version some/image"},

				{"detector: only -version is present", "detector -version"},
				{"detector: other params are set", "detector -app=/some/dir -version"},

				{"exporter: only -version is present", "exporter -version"},
				{"exporter: other params are set", "exporter -analyzed=/some/file -version some/image"},

				{"restorer: only -version is present", "restorer -version"},
				{"restorer: other params are set", "restorer -cache-dir=/some/dir -version"},
			} {
				desc := data[0]
				binary, args := parseCommand(data[1])

				when(desc, func() {
					it("only prints the version", func() {
						output, err := lifecycleCmd(binary, args...).CombinedOutput()
						if err != nil {
							t.Error(err)
						}

						h.AssertStringContains(t, string(output), "some-version+asdf123")
					})
				})
			}
		})
	})
}

func parseCommand(command string) (binary string, args []string) {
	parts := strings.Split(command, " ")
	return parts[0], parts[1:]
}

func buildBinaries(t *testing.T, dir string) {
	cmd := exec.Command("make", "build-"+runtime.GOOS)
	cmd.Dir = ".."
	cmd.Env = append(os.Environ(), "BUILD_DIR="+dir, "LIFECYCLE_VERSION=some-version", "SCM_COMMIT=asdf123")

	t.Log("Building binaries: ", cmd.Args)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to run %v\n OUTPUT: %s\n ERROR: %s\n", cmd.Args, output, err)
	}
}
