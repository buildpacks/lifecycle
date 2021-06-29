package acceptance

import (
	"fmt"
	"math/rand"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	builderDockerContext = filepath.Join("testdata", "builder")
	builderBinaryDir     = filepath.Join("testdata", "builder", "container", "cnb", "lifecycle")
	builderImage         = "lifecycle/acceptance/builder"
	builderUserID        = "1234"
)

func TestBuilder(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "builder acceptance tests are not yet supported on Windows")

	rand.Seed(time.Now().UTC().UnixNano())

	h.MakeAndCopyLifecycle(t, "linux", builderBinaryDir)
	h.DockerBuild(t,
		builderImage,
		builderDockerContext,
		h.WithArgs("--build-arg", fmt.Sprintf("cnb_platform_api=%s", api.Platform.Latest())),
	)
	defer h.DockerImageRemove(t, builderImage)

	spec.Run(t, "acceptance-builder", testBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	// .../cmd/lifecycle/builder.go#45
	when("called with arguments", func() {
		it("errors", func() {
			command := exec.Command(
				"docker",
				"run",
				"--rm",
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				builderImage,
				"some-arg",
			)
			output, err := command.CombinedOutput()
			h.AssertNotNil(t, err)
			expected := "failed to parse arguments: received unexpected arguments"
			h.AssertStringContains(t, string(output), expected)
		})
	})

	// .../cmd/lifecycle/builder.go#62
	when("running as a root", func() {
		it("errors", func() {
			command := exec.Command(
				"docker",
				"run",
				"--rm",
				"--user",
				"root",
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				builderImage,
			)
			output, err := command.CombinedOutput()
			h.AssertNotNil(t, err)
			expected := "failed to build: refusing to run as root"
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("read buildpack group file", func() {
		it("no default group toml file in default location", func() {
			command := exec.Command(
				"docker",
				"run",
				"--rm",
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				builderImage,
			)
			output, err := command.CombinedOutput()
			h.AssertNotNil(t, err)
			expected := "failed to read buildpack group: open /layers/group.toml: no such file or directory"
			h.AssertStringContains(t, string(output), expected)
		})

		//TODO: check some output file for this case not except any error message
		it("empty group toml file", func() {
			command := exec.Command(
				"docker",
				"run",
				"--rm",
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				"--env", "CNB_GROUP_PATH=/cnb/group_tomls/empty_group.toml",
				builderImage,
			)
			_, err := command.CombinedOutput()
			//print(string(output), err)
			h.AssertNil(t, err)
			//expected := "failed to read buildpack order file"
			//h.AssertStringContains(t, string(output), expected)
		})

		it("invalid group toml file", func() {
			command := exec.Command(
				"docker",
				"run",
				"--rm",
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				"--env", "CNB_GROUP_PATH=/cnb/group_tomls/wrong_group.toml",
				builderImage,
			)
			output, err := command.CombinedOutput()
			h.AssertNotNil(t, err)
			expected := "failed to read buildpack group: Near line"
			h.AssertStringContains(t, string(output), expected)
		})
	})
}
