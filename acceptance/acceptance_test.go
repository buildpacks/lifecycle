package acceptance

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

const (
	expectedVersion = "some-version"
	expectedCommit  = "asdf123"
)

var (
	latestPlatformAPI = platform.APIs.Latest().String()
	buildDir          string
)

func TestVersion(t *testing.T) {
	var err error
	buildDir, err = ioutil.TempDir("", "lifecycle-acceptance")
	h.AssertNil(t, err)
	defer func() {
		h.AssertNil(t, os.RemoveAll(buildDir))
	}()

	outDir := filepath.Join(buildDir, fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH), "lifecycle")
	h.AssertNil(t, os.MkdirAll(outDir, 0755))

	h.MakeAndCopyLifecycle(t,
		runtime.GOOS,
		runtime.GOARCH,
		outDir,
		"LIFECYCLE_VERSION=some-version",
		"SCM_COMMIT="+expectedCommit,
	)
	spec.Run(t, "acceptance", testVersion, spec.Parallel(), spec.Report(report.Terminal{}))
}

type testCase struct {
	description string
	focus       bool
	command     string
	args        []string
}

func testVersion(t *testing.T, when spec.G, it spec.S) {
	when("All", func() {
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
				w := when
				if tc.focus {
					w = when.Focus
				}
				w(tc.description, func() {
					it("only prints the version", func() {
						cmd := lifecycleCmd(tc.command, tc.args...)
						output, err := cmd.CombinedOutput()
						if err != nil {
							t.Fatalf("failed to run %v\n OUTPUT: %s\n ERROR: %s\n", cmd.Args, output, err)
						}
						h.AssertStringContains(t, string(output), expectedVersion+"+"+expectedCommit)
					})
				})
			}
		})
	})
}

func lifecycleCmd(phase string, args ...string) *exec.Cmd {
	return exec.Command(filepath.Join(buildDir, fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH), "lifecycle", phase), args...) // #nosec G204
}
