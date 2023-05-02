package acceptance

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	builderDockerContext               = filepath.Join("testdata", "builder")
	builderBinaryDir                   = filepath.Join("testdata", "builder", "container", "cnb", "lifecycle")
	builderImage                       = "lifecycle/acceptance/builder"
	builderDaemonOS, builderDaemonArch string
)

func TestBuilder(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "Builder acceptance tests are not yet supported on Windows")
	h.SkipIf(t, runtime.GOARCH != "amd64", "Builder acceptance tests are not yet supported on non-amd64")

	rand.Seed(time.Now().UTC().UnixNano())

	info, err := h.DockerCli(t).Info(context.TODO())
	h.AssertNil(t, err)

	// These variables are clones of the variables in analyzer_test.go.
	// You can find the same variables there without `builder` prefix.
	// These lines are added for supporting windows tests.
	builderDaemonOS = info.OSType
	builderDaemonArch = info.Architecture
	if builderDaemonArch == "x86_64" {
		builderDaemonArch = "amd64"
	}

	h.MakeAndCopyLifecycle(t, builderDaemonOS, builderDaemonArch, builderBinaryDir)
	h.DockerBuild(t,
		builderImage,
		builderDockerContext,
		h.WithArgs("--build-arg", fmt.Sprintf("cnb_platform_api=%s", api.Platform.Latest())),
		h.WithFlags(
			"-f", filepath.Join(builderDockerContext, dockerfileName),
		),
	)
	defer h.DockerImageRemove(t, builderImage)

	spec.Run(t, "acceptance-builder", testBuilder, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	var copyDir, containerName, cacheVolume string

	it.Before(func() {
		containerName = "test-container-" + h.RandString(10)
		var err error
		copyDir, err = os.MkdirTemp("", "test-docker-copy-")
		h.AssertNil(t, err)
	})

	it.After(func() {
		if h.DockerContainerExists(t, containerName) {
			h.Run(t, exec.Command("docker", "rm", containerName))
		}
		if h.DockerVolumeExists(t, cacheVolume) {
			h.DockerVolumeRemove(t, cacheVolume)
		}
		os.RemoveAll(copyDir)
	})

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

	when("correct and full group.toml and plan.toml", func() {
		it("succeeds", func() {
			h.DockerRunAndCopy(t,
				containerName,
				copyDir,
				ctrPath("/layers"),
				builderImage,
				h.WithFlags(
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
				),
			)
			// check builder metadata.toml for success test
			_, md := getBuilderMetadata(t, filepath.Join(copyDir, "layers", "config", "metadata.toml"))

			h.AssertStringContains(t, md.Buildpacks[0].API, "0.2")
			h.AssertStringContains(t, md.Buildpacks[0].ID, "hello_world")
			h.AssertStringContains(t, md.Buildpacks[0].Version, "0.0.1")
		})
	})

	when("writing metadata.toml", func() {
		it("writes and reads successfully", func() {
			h.DockerRunAndCopy(t,
				containerName,
				copyDir,
				ctrPath("/layers"),
				builderImage,
				h.WithFlags(
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
				),
			)
			// check builder metadata.toml for success test
			contents, md := getBuilderMetadata(t, filepath.Join(copyDir, "layers", "config", "metadata.toml"))

			// prevent regression of inline table serialization
			h.AssertStringDoesNotContain(t, contents, "processes =")
			h.AssertStringContains(t, md.Buildpacks[0].API, "0.2")
			h.AssertStringContains(t, md.Buildpacks[0].ID, "hello_world")
			h.AssertStringContains(t, md.Buildpacks[0].Version, "0.0.1")
			h.AssertEq(t, len(md.Processes), 1)
			h.AssertEq(t, md.Processes[0].Type, "hello")
			h.AssertEq(t, len(md.Processes[0].Command.Entries), 1)
			h.AssertEq(t, md.Processes[0].Command.Entries[0], "echo world")
			h.AssertEq(t, len(md.Processes[0].Args), 1)
			h.AssertEq(t, md.Processes[0].Args[0], "arg1")
			h.AssertEq(t, md.Processes[0].Direct, false)
			h.AssertEq(t, md.Processes[0].WorkingDirectory, "")
			h.AssertEq(t, md.Processes[0].Default, false)
		})

		when("the platform < 0.10", func() {
			it("writes and reads successfully", func() {
				h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					ctrPath("/layers"),
					builderImage,
					h.WithFlags(
						"--env", "CNB_PLATFORM_API=0.9",
						"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
						"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
					),
				)
				// check builder metadata.toml for success test
				contents, md := getBuilderMetadata(t, filepath.Join(copyDir, "layers", "config", "metadata.toml"))

				// prevent regression of inline table serialization
				h.AssertStringDoesNotContain(t, contents, "processes =")
				h.AssertStringContains(t, md.Buildpacks[0].API, "0.2")
				h.AssertStringContains(t, md.Buildpacks[0].ID, "hello_world")
				h.AssertStringContains(t, md.Buildpacks[0].Version, "0.0.1")
				h.AssertEq(t, len(md.Processes), 1)
				h.AssertEq(t, md.Processes[0].Type, "hello")
				h.AssertEq(t, len(md.Processes[0].Command.Entries), 1)
				h.AssertEq(t, md.Processes[0].Command.Entries[0], "echo world")
				h.AssertEq(t, len(md.Processes[0].Args), 1)
				h.AssertEq(t, md.Processes[0].Args[0], "arg1")
				h.AssertEq(t, md.Processes[0].Direct, false)
				h.AssertEq(t, md.Processes[0].WorkingDirectory, "")
				h.AssertEq(t, md.Processes[0].Default, false)
			})
		})
	})

	when("-group contains extensions", func() {
		it("includes the provided extensions in <layers>/config/metadata.toml", func() {
			h.DockerRunAndCopy(t,
				containerName,
				copyDir,
				ctrPath("/layers"),
				builderImage,
				h.WithFlags(
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					"--env", "CNB_GROUP_PATH=/cnb/group_tomls/group_with_ext.toml",
					"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
				),
			)
			// check builder metadata.toml for success test
			_, md := getBuilderMetadata(t, filepath.Join(copyDir, "layers", "config", "metadata.toml"))

			h.AssertStringContains(t, md.Buildpacks[0].API, "0.2")
			h.AssertStringContains(t, md.Buildpacks[0].ID, "hello_world")
			h.AssertStringContains(t, md.Buildpacks[0].Version, "0.0.1")
			h.AssertStringContains(t, md.Extensions[0].API, "0.9")
			h.AssertEq(t, md.Extensions[0].Extension, false) // this shows that `extension = true` is not redundantly printed in group.toml
			h.AssertStringContains(t, md.Extensions[0].ID, "hello_world")
			h.AssertStringContains(t, md.Extensions[0].Version, "0.0.1")
		})
	})

	when("invalid input files", func() {
		// .../cmd/lifecycle/builder.go#readData
		when("group.toml", func() {
			when("not found", func() {
				it("errors", func() {
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
					expected := "failed to read buildpack group: open /layers/group.toml: no such file or directory"
					h.AssertStringContains(t, string(output), expected)
				})
			})

			when("empty", func() {
				it("succeeds", func() {
					h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						ctrPath("/layers"),
						builderImage,
						h.WithFlags(
							"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
							"--env", "CNB_GROUP_PATH=/cnb/group_tomls/empty_group.toml",
							"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
						),
					)
					// check builder metadata.toml for success test
					_, md := getBuilderMetadata(t, filepath.Join(copyDir, "layers", "config", "metadata.toml"))
					h.AssertEq(t, len(md.Processes), 0)
				})
			})

			when("invalid", func() {
				it("errors", func() {
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
					expected := "failed to read buildpack group: toml: line 1: expected '.' or '=', but got 'a' instead"
					h.AssertStringContains(t, string(output), expected)
				})
			})

			// .../cmd/lifecycle/builder.go#Exec
			when("invalid builpack api", func() {
				it("errors", func() {
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
					expected := "parse buildpack API '<nil>' for buildpack 'hello_world@0.0.1'"
					h.AssertStringContains(t, string(output), expected)
				})
			})
		})

		// .../cmd/lifecycle/builder.go#readData
		when("plan.toml", func() {
			when("not found", func() {
				it("errors", func() {
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
					expected := "failed to parse detect plan: open /layers/plan.toml: no such file or directory"
					h.AssertStringContains(t, string(output), expected)
				})
			})

			when("empty", func() {
				it("succeeds", func() {
					h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						ctrPath("/layers"),
						builderImage,
						h.WithFlags(
							"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
							"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/empty_plan.toml",
							"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
						),
					)
					// check builder metadata.toml for success test
					_, md := getBuilderMetadata(t, filepath.Join(copyDir, "layers", "config", "metadata.toml"))

					h.AssertStringContains(t, md.Buildpacks[0].API, "0.2")
					h.AssertStringContains(t, md.Buildpacks[0].ID, "hello_world")
					h.AssertStringContains(t, md.Buildpacks[0].Version, "0.0.1")
				})
			})

			when("invalid", func() {
				it("errors", func() {
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
					expected := "failed to parse detect plan: toml: line 1: expected '.' or '=', but got 'a' instead"
					h.AssertStringContains(t, string(output), expected)
				})
			})
		})
	})

	when("determining the location of input files", func() {
		// .../cmd/lifecycle/builder.go#Args
		when("group.toml path is not specified", func() {
			it("will look for group.toml in the provided layers directory", func() {
				h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					ctrPath("/layers"),
					builderImage,
					h.WithFlags(
						"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
						"--env", "CNB_LAYERS_DIR=/layers/different_layer_dir_from_env",
						"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan_buildpack_2.toml",
					),
				)
				_, md := getBuilderMetadata(t, filepath.Join(copyDir, "layers/different_layer_dir_from_env/config/metadata.toml"))

				h.AssertStringContains(t, md.Buildpacks[0].API, "0.2")
				h.AssertStringContains(t, md.Buildpacks[0].ID, "hello_world_2")
				h.AssertStringContains(t, md.Buildpacks[0].Version, "0.0.2")
			})
		})

		// .../cmd/lifecycle/builder.go#Args
		when("plan.toml path is not specified", func() {
			it("will look for plan.toml in the provided layers directory", func() {
				h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					ctrPath("/layers"),
					builderImage,
					h.WithFlags(
						"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
						"--env", "CNB_LAYERS_DIR=/layers/different_layer_dir_from_env",
						"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group_buildpack2.toml",
					),
				)
				_, md := getBuilderMetadata(t, filepath.Join(copyDir, "layers/different_layer_dir_from_env/config/metadata.toml"))

				h.AssertStringContains(t, md.Buildpacks[0].API, "0.2")
				h.AssertStringContains(t, md.Buildpacks[0].ID, "hello_world_2")
				h.AssertStringContains(t, md.Buildpacks[0].Version, "0.0.2")
			})
		})
	})

	when("CNB_APP_DIR is set", func() {
		it("sets the buildpacks' working directory to CNB_APP_DIR", func() {
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
			h.AssertNil(t, err)
			expected := "CNB_APP_DIR: /env_folders/different_cnb_app_dir_from_env"
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("CNB_BUILDPACKS_DIR is set", func() {
		it("uses buildpacks from CNB_BUILDPACKS_DIR", func() {
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
			h.AssertNil(t, err)
			expected := "CNB_BUILDPACK_DIR: /env_folders/different_buildpack_dir_from_env"
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("CNB_LAYERS_DIR is set", func() {
		it("CNB_LAYERS_DIR is a parent of the buildpack layers dir", func() {
			command := exec.Command(
				"docker",
				"run",
				"--rm",
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
				"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
				"--env", "CNB_LAYERS_DIR=/layers/different_layer_dir_from_env",
				builderImage,
			)
			output, err := command.CombinedOutput()
			h.AssertNil(t, err)
			expected := "LAYERS_DIR: /layers/different_layer_dir_from_env/hello_world"
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("CNB_PLAN_PATH is set", func() {
		it("provides the buildpack a filtered version of the plan found at CNB_PLAN_PATH", func() {
			command := exec.Command(
				"docker",
				"run",
				"--rm",
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
				"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/different_plan_from_env.toml",
				builderImage,
			)
			output, err := command.CombinedOutput()
			h.AssertNil(t, err)
			expected := "name = \"different_plan_from_env.toml_reqires_subset_content\""
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("CNB_PLATFORM_DIR is set", func() {
		it("CNB_PLATFORM_DIR is successfully transmitted to build script", func() {
			command := exec.Command(
				"docker",
				"run",
				"--rm",
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				"--env", "CNB_GROUP_PATH=/cnb/group_tomls/always_detect_group.toml",
				"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan.toml",
				"--env", "CNB_PLATFORM_DIR=/env_folders/different_platform_dir_from_env",
				builderImage,
			)
			output, err := command.CombinedOutput()
			h.AssertNil(t, err)
			expected := "PLATFORM_DIR: /env_folders/different_platform_dir_from_env"
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("It runs", func() {
		it("sets CNB_TARGET_* vars", func() {
			command := exec.Command(
				"docker",
				"run",
				"--rm",
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				"--env", "CNB_LAYERS_DIR=/layers/03_layer",
				"--env", "CNB_PLAN_PATH=/cnb/plan_tomls/always_detect_plan_buildpack_3.toml",
				builderImage,
			)
			output, err := command.CombinedOutput()
			fmt.Println(string(output))
			h.AssertNil(t, err)
			h.AssertStringContains(t, string(output), "CNB_TARGET_ARCH: amd64")
			h.AssertStringContains(t, string(output), "CNB_TARGET_OS: linux")
			h.AssertStringContains(t, string(output), "CNB_TARGET_VARIANT: some-variant")
			h.AssertStringContains(t, string(output), "CNB_TARGET_DISTRO_NAME: ubuntu")
			h.AssertStringContains(t, string(output), "CNB_TARGET_DISTRO_VERSION: some-cute-version")
		})
	})
}

func getBuilderMetadata(t *testing.T, path string) (string, *platform.BuildMetadata) {
	t.Helper()
	contents, _ := os.ReadFile(path)
	h.AssertEq(t, len(contents) > 0, true)

	var buildMD platform.BuildMetadata
	_, err := toml.Decode(string(contents), &buildMD)
	h.AssertNil(t, err)

	return string(contents), &buildMD
}
