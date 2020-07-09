// +build acceptance

package acceptance

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	h "github.com/buildpacks/lifecycle/testhelpers"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

var (
	detectDockerContext = filepath.Join("testdata", "detector")
	detectorBinaryDir    = filepath.Join("testdata", "detector", "container", "cnb", "lifecycle")
	detectImage         = "lifecycle/acceptance/detector"
	userID = "1234"
)

func TestDetector(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "Detector is not yet supported on Windows")

	rand.Seed(time.Now().UTC().UnixNano())

	h.MakeAndCopyLifecycle(t, "linux", detectorBinaryDir)
	h.DockerBuild(t, detectImage, detectDockerContext)
	defer h.DockerImageRemove(t, detectImage)

	spec.Run(t, "acceptance-detector", testDetector, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testDetector(t *testing.T, when spec.G, it spec.S) {
	when("called with arguments", func() {
		it("errors", func() {
			cmd := exec.Command("docker", "run", "--rm", detectImage, "some-arg")
			output, err := cmd.CombinedOutput()
			h.AssertNotNil(t, err)
			expected := "failed to parse arguments: received unexpected arguments"
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("running as a root", func() {
		it("errors", func() {
			cmd := exec.Command("docker", "run", "--rm", detectImage)
			output, err := cmd.CombinedOutput()
			h.AssertNotNil(t, err)
			expected := "failed to build: refusing to run as root"
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("read buildpack order file failed", func() {
		it("errors", func() {
			// no order.toml file in the default directory
			cmd := exec.Command("docker", "run", "--rm", "--user", userID, detectImage)
			output, err := cmd.CombinedOutput()
			h.AssertNotNil(t, err)
			expected := "failed to read buildpack order file"
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("no buildpack group passed detection", func() {
		it("errors", func() {
			cmd := exec.Command("docker", "run", "--rm", "--user", userID, "--env", "CNB_ORDER_PATH=/cnb/orders/empty_order.toml", detectImage)
			output, err := cmd.CombinedOutput()
			h.AssertNotNil(t, err)
			expected := "No buildpack groups passed detection."
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("there is a buildpack group that pass detection", func() {
		it("writes group.toml and plan.toml", func() {
			containerName := "test-container-" + h.RandString(10)
			h.Run(t, exec.Command(
				"docker",
				"run",
				"--name", containerName,
				"--user", userID,
				"--env", "CNB_ORDER_PATH=/cnb/orders/simple_order.toml",
				detectImage))
			defer h.Run(t, exec.Command("docker", "rm", containerName))

			// check group.toml
			tempGroupToml, err := ioutil.TempFile("", "")
			h.AssertNil(t, err)
			defer os.RemoveAll(tempGroupToml.Name())

			h.Run(t, exec.Command(
				"docker", "cp",
				fmt.Sprintf("%s:/layers/group.toml", containerName),
				tempGroupToml.Name(),
			))

			groupContents, err := ioutil.ReadFile(tempGroupToml.Name())
			h.AssertNil(t, err)
			h.AssertEq(t, len(groupContents) > 0, true)
			expected := "[[group]]\n  id = \"simple_buildpack\"\n  version = \"simple_buildpack_version\""
			h.AssertStringContains(t, string(groupContents), expected)

			// check plan.toml - should be empty
			tempPlanToml, err := ioutil.TempFile("", "")
			h.AssertNil(t, err)
			defer os.RemoveAll(tempPlanToml.Name())

			h.Run(t, exec.Command(
				"docker", "cp",
				fmt.Sprintf("%s:/layers/plan.toml", containerName),
				tempPlanToml.Name(),
			))

			planContents, err := ioutil.ReadFile(tempPlanToml.Name())
			h.AssertNil(t, err)
			h.AssertEq(t, len(planContents) > 0, true)
			expected = "[[entries]]\n\n  [[entries.providers]]\n    id = \"simple_buildpack\"\n    version = \"simple_buildpack_version\"\n\n  [[entries.requires]]\n    name = \"some-world\"\n    version = \"0.1\"\n    [entries.requires.metadata]\n      world = \"Earth-616\""
			h.AssertStringContains(t, string(planContents), expected)
		})
	})

	when("environment variables are provided for buildpack and app directories and for the output files", func() {
		it("writes group.toml and plan.toml in the right location and with the right names", func() {
			containerName := "test-container-" + h.RandString(10)
			h.Run(t, exec.Command(
				"docker",
				"run",
				"--name", containerName,
				"--user", userID,
				"--env", "CNB_ORDER_PATH=/cnb/orders/always_detect_order.toml",
				"--env", "CNB_BUILDPACKS_DIR=/cnb/custom_buildpacks",
				"--env", "CNB_APP_DIR=/custom_workspace",
				"--env", "CNB_GROUP_PATH=./custom_group.toml",
				"--env", "CNB_PLAN_PATH=./custom_plan.toml",
				"--env", "CNB_PLATFORM_DIR=/custom_platform",
				detectImage,
				"-log-level=debug"))
			defer h.Run(t, exec.Command("docker", "rm", containerName))

			// check group.toml
			tempGroupToml, err := ioutil.TempFile("", "")
			h.AssertNil(t, err)
			defer os.RemoveAll(tempGroupToml.Name())

			h.Run(t, exec.Command(
				"docker", "cp",
				fmt.Sprintf("%s:/layers/custom_group.toml", containerName),
				tempGroupToml.Name(),
			))

			groupContents, err := ioutil.ReadFile(tempGroupToml.Name())
			h.AssertNil(t, err)
			h.AssertEq(t, len(groupContents) > 0, true)
			expectedGroupContents := "[[group]]\n  id = \"always_detect_buildpack\"\n  version = \"always_detect_buildpack_version\""
			h.AssertStringContains(t, string(groupContents), expectedGroupContents)

			// check plan.toml - should be empty
			tempPlanToml, err := ioutil.TempFile("", "")
			h.AssertNil(t, err)
			defer os.RemoveAll(tempPlanToml.Name())

			h.Run(t, exec.Command(
				"docker", "cp",
				fmt.Sprintf("%s:/layers/custom_plan.toml", containerName),
				tempPlanToml.Name(),
			))

			planContents, err := ioutil.ReadFile(tempPlanToml.Name())
			h.AssertNil(t, err)
			h.AssertEq(t, len(planContents) == 0, true)

			// check platform directory
			containerId := strings.TrimSuffix(h.Run(t, exec.Command("docker", "ps", "-aqf", fmt.Sprintf("name=%s", containerName))),"\n")
			logs := h.Run(t, exec.Command("docker", "logs", containerId))
			expectedLogs := "platform_path: /custom_platform"
			h.AssertStringContains(t, string(logs), expectedLogs)
		})
	})
}

func buildDetector(t *testing.T) {
	cmd := exec.Command("make", "clean", "build-linux-lifecycle")
	wd, err := os.Getwd()
	h.AssertNil(t, err)
	cmd.Dir = filepath.Join(wd, "..")
	cmd.Env = append(
		os.Environ(),
		"PWD="+cmd.Dir,
		"OUT_DIR="+detectorBinaryDir,
		"LIFECYCLE_VERSION=some-version",
		"SCM_COMMIT=asdf123",
	)

	t.Log("Building binaries: ", cmd.Args)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to run %v\n OUTPUT: %s\n ERROR: %s\n", cmd.Args, output, err)
	}
}
