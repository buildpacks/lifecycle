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
	// FIXME: try other OS, should be fine
	h.SkipIf(t, runtime.GOOS == "windows", "builder acceptance tests are not yet supported on Windows")

	rand.Seed(time.Now().UTC().UnixNano())

	// FIXME: this is for development speed we need to comment out before production !
	//h.MakeAndCopyLifecycle(t, "linux", builderBinaryDir)
	h.DockerBuild(t,
		builderImage,
		builderDockerContext,
		h.WithArgs("--build-arg", fmt.Sprintf("cnb_platform_api=%s", api.Platform.Latest())),
	)
	// FIXME: this is for development speed we need to comment out before production !
	//defer h.DockerImageRemove(t, builderImage)

	spec.Run(t, "acceptance-builder", testBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	// .../cmd/lifecycle/builder.go#Args
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

	// .../cmd/lifecycle/builder.go#Privileges
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

	when("error on reading Data", func() {

		// .../cmd/lifecycle/builder.go#readData
		when("read buildpack group file", func() {
			it("no default group toml file in default location", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
					builderImage,
				)
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "failed to read buildpack group"
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
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
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
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
					builderImage,
				)
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "failed to read buildpack group: Near line"
				h.AssertStringContains(t, string(output), expected)
			})

			// .../cmd/lifecycle/builder.go#Exec
			it("invalid builpack api group toml file", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_GROUP_PATH=/cnb/group_tomls/invalid_buildpack_api_group.toml",
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
					builderImage,
				)
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "failed to : parse buildpack API"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		// .../cmd/lifecycle/builder.go#readData
		when("error during parse plan", func() {
			it("no default plan.toml file in default location", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
					builderImage,
				)
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "failed to parse detect plan"
				h.AssertStringContains(t, string(output), expected)
			})

			//TODO: check some output file for this case not except any error message
			it("empty parse plan.toml file", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/empty_plan.toml",
					"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
					builderImage,
				)
				_, err := command.CombinedOutput()
				//print(string(output), err)
				h.AssertNil(t, err)
				//expected := "failed to read buildpack order file"
				//h.AssertStringContains(t, string(output), expected)
			})

			it("invalid parse plan.toml file", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/wrong_plan.toml",
					"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
					builderImage,
				)
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "failed to parse detect plan: Near line"
				h.AssertStringContains(t, string(output), expected)
			})

		})
	})

	when("PlaceHolders need to replace with defaults", func() {

		// .../cmd/lifecycle/builder.go#Args
		when("groupPath is equals to PlaceHolder groupPath", func() {
			it("will replace placeholder with default groupPath", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_GROUP_PATH=<layers>/group.toml",
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
					builderImage,
				)
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "failed to read buildpack group: open /layers/group.toml"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		// .../cmd/lifecycle/builder.go#Args
		when("planPath is equals to PlaceHolder planPath", func() {
			it("will replace placeholder with default planPath", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_PLAN_PATH=<layers>/plan.toml",
					"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
					builderImage,
				)
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "failed to parse detect plan: open /layers/plan.toml"
				h.AssertStringContains(t, string(output), expected)
			})
		})

	})

	/// .../cmd/lifecycle/builder.go#build
	when("Builder args are successfully transmitted to in build script", func() {
		when("CNB_APP_DIR changed", func() {
			it("CNB_APP_DIR is successfully transmitted to in build script", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
					"--env", "CNB_APP_DIR=/env_folders/different_cnb_app_dir_from_env",
					builderImage,
				)
				output, err := command.CombinedOutput()
				//print(string(output), err)
				h.AssertNil(t, err) //due to not exist directory
				expected := "CNB_APP_DIR: /env_folders/different_cnb_app_dir_from_env"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("CNB_BUILDPACKS_DIR changed", func() {
			it("CNB_BUILDPACKS_DIR is successfully transmitted to in build script", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
					"--env", "CNB_BUILDPACKS_DIR=/env_folders/different_buildpack_dir_from_env",
					builderImage,
				)
				output, err := command.CombinedOutput()
				//print(string(output), err)
				h.AssertNil(t, err) //due to not exist directory
				expected := "CNB_BUILDPACK_DIR: /env_folders/different_buildpack_dir_from_env"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("CNB_LAYERS_DIR", func() {
			it("CNB_LAYERS_DIR is successfully transmitted to in build script", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
					"--env", "CNB_LAYERS_DIR=/tmp/different_layers_path_dir_from_env",
					builderImage,
				)
				output, err := command.CombinedOutput()
				//print(string(output), err)
				h.AssertNil(t, err) //due to not exist directory
				expected := "layers_dir: /tmp/different_layers_path_dir_from_env"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("CNB_PLAN_PATH", func() {
			it.Focus("CNB_PLAN_PATH is successfully transmitted to in build script", func() {

				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/differrent_plan_from_env.toml",
					builderImage,
				)
				output, err := command.CombinedOutput()
				print(string(output), err)
				h.AssertNil(t, err) //due to not exist directory
				expected := "different_plan_from_env.toml_reqires_subset_content"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("CNB_PLATFORM_DIR", func() {
			it("CNB_PLATFORM_DIR is successfully transmitted to in build script", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
					"--env", "CNB_PLATFORM_DIR=/different_platform_dir_from_env",
					builderImage,
				)
				output, err := command.CombinedOutput()
				//print(string(output), err)
				h.AssertNotNil(t, err) //due to not exist directory
				expected := "/different_platform_dir_from_env"
				h.AssertStringContains(t, string(output), expected)
			})
		})
	})

}
