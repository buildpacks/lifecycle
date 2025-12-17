//go:build acceptance

package acceptance

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/moby/moby/client"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	detectDockerContext                  = filepath.Join("testdata", "detector")
	detectorBinaryDir                    = filepath.Join("testdata", "detector", "container", "cnb", "lifecycle")
	detectImage                          = "lifecycle/acceptance/detector"
	userID                               = "1234"
	detectorDaemonOS, detectorDaemonArch string
)

func TestDetector(t *testing.T) {
	info, err := h.DockerCli(t).Info(context.TODO(), client.InfoOptions{})
	h.AssertNil(t, err)

	detectorDaemonOS = info.Info.OSType
	detectorDaemonArch = info.Info.Architecture
	if detectorDaemonArch == "x86_64" {
		detectorDaemonArch = "amd64"
	}
	if detectorDaemonArch == "aarch64" {
		detectorDaemonArch = "arm64"
	}

	h.MakeAndCopyLifecycle(t, detectorDaemonOS, detectorDaemonArch, detectorBinaryDir)
	h.DockerBuild(t,
		detectImage,
		detectDockerContext,
		h.WithArgs("--build-arg", fmt.Sprintf("cnb_platform_api=%s", api.Platform.Latest())),
	)
	t.Cleanup(func() { h.DockerImageRemoveSafe(t, detectImage) })

	for _, platformAPI := range api.Platform.Supported {
		if platformAPI.LessThan("0.12") {
			continue
		}

		spec.Run(t, "acceptance-detector/"+platformAPI.String(), testDetectorFunc(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testDetectorFunc(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		when("called with arguments", func() {
			it("errors", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+platformAPI,
					detectImage,
					"some-arg",
				)
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "failed to parse arguments: received unexpected arguments"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("running as a root", func() {
			it("errors", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--user",
					"root",
					"--env", "CNB_PLATFORM_API="+platformAPI,
					detectImage,
				)
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "failed to detect: refusing to run as root"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("read buildpack order file failed", func() {
			it("errors", func() {
				// no order.toml file in the default search locations
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_PLATFORM_API="+platformAPI,
					detectImage,
				)
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "failed to initialize detector: reading order"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("no buildpack group passed detection", func() {
			it("errors and exits with the expected code", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_ORDER_PATH=/cnb/orders/fail_detect_order.toml",
					"--env", "CNB_PLATFORM_API="+platformAPI,
					detectImage,
				)
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				failErr, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("expected an error of type exec.ExitError")
				}
				h.AssertEq(t, failErr.ExitCode(), 20) // platform code for failed detect

				expected1 := `======== Output: fail_detect_buildpack@some_version ========
Opted out of detection
======== Results ========
fail: fail_detect_buildpack@some_version`
				h.AssertStringContains(t, string(output), expected1)
				expected2 := "No buildpack groups passed detection."
				h.AssertStringContains(t, string(output), expected2)
			})
		})

		when("there is a buildpack group that passes detection", func() {
			var copyDir, containerName string

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
				os.RemoveAll(copyDir)
			})

			it("writes group.toml and plan.toml at the default locations", func() {
				output := h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					"/layers",
					detectImage,
					h.WithFlags("--user", userID,
						"--env", "CNB_ORDER_PATH=/cnb/orders/simple_order.toml",
						"--env", "CNB_PLATFORM_API="+platformAPI,
					),
					h.WithArgs(),
				)

				// check group.toml
				foundGroupTOML := filepath.Join(copyDir, "layers", "group.toml")
				group, err := files.Handler.ReadGroup(foundGroupTOML)
				h.AssertNil(t, err)
				h.AssertEq(t, group.Group[0].ID, "simple_buildpack")
				h.AssertEq(t, group.Group[0].Version, "simple_buildpack_version")

				// check plan.toml
				foundPlanTOML := filepath.Join(copyDir, "layers", "plan.toml")
				buildPlan, err := files.Handler.ReadPlan(foundPlanTOML)
				h.AssertNil(t, err)
				h.AssertEq(t, buildPlan.Entries[0].Providers[0].ID, "simple_buildpack")
				h.AssertEq(t, buildPlan.Entries[0].Providers[0].Version, "simple_buildpack_version")
				h.AssertEq(t, buildPlan.Entries[0].Requires[0].Name, "some_requirement")
				h.AssertEq(t, buildPlan.Entries[0].Requires[0].Metadata["some_metadata_key"], "some_metadata_val")
				h.AssertEq(t, buildPlan.Entries[0].Requires[0].Metadata["version"], "some_version")

				// check output
				h.AssertStringContains(t, output, "simple_buildpack simple_buildpack_version")
				h.AssertStringDoesNotContain(t, output, "======== Results ========") // log output is info level as detect passed
			})
		})

		when("environment variables are provided for buildpack and app directories and for the output files", func() {
			var copyDir, containerName string

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
				os.RemoveAll(copyDir)
			})

			it("writes group.toml and plan.toml in the right locations and with the right names", func() {
				h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					"/layers",
					detectImage,
					h.WithFlags("--user", userID,
						"--env", "CNB_ORDER_PATH=/cnb/orders/always_detect_order.toml",
						"--env", "CNB_BUILDPACKS_DIR=/cnb/custom_buildpacks",
						"--env", "CNB_APP_DIR=/custom_workspace",
						"--env", "CNB_GROUP_PATH=./custom_group.toml",
						"--env", "CNB_PLAN_PATH=./custom_plan.toml",
						"--env", "CNB_PLATFORM_DIR=/custom_platform",
						"--env", "CNB_PLATFORM_API="+platformAPI,
					),
					h.WithArgs("-log-level=debug"),
				)

				// check group.toml
				foundGroupTOML := filepath.Join(copyDir, "layers", "custom_group.toml")
				group, err := files.Handler.ReadGroup(foundGroupTOML)
				h.AssertNil(t, err)
				h.AssertEq(t, group.Group[0].ID, "always_detect_buildpack")
				h.AssertEq(t, group.Group[0].Version, "always_detect_buildpack_version")

				// check plan.toml - should be empty since we're using always_detect_order.toml so there is no "actual plan"
				tempPlanToml := filepath.Join(copyDir, "layers", "custom_plan.toml")
				planContents, err := os.ReadFile(tempPlanToml)
				h.AssertNil(t, err)
				h.AssertEq(t, len(planContents) == 0, true)

				// check platform directory
				logs := h.Run(t, exec.Command("docker", "logs", containerName))
				expectedPlatformPath := "platform_path: /custom_platform"
				expectedAppDir := "app_dir: /custom_workspace"
				h.AssertStringContains(t, logs, expectedPlatformPath)
				h.AssertStringContains(t, logs, expectedAppDir)
			})
		})

		when("-order is provided", func() {
			var copyDir, containerName, expectedOrderTOMLPath string

			it.Before(func() {
				containerName = "test-container-" + h.RandString(10)
				var err error
				copyDir, err = os.MkdirTemp("", "test-docker-copy-")
				h.AssertNil(t, err)

				simpleOrderTOML := filepath.Join("testdata", "detector", "container", "cnb", "orders", "simple_order.toml")
				expectedOrderTOMLPath, err = filepath.Abs(simpleOrderTOML)
				h.AssertNil(t, err)
			})

			it.After(func() {
				if h.DockerContainerExists(t, containerName) {
					h.Run(t, exec.Command("docker", "rm", containerName))
				}
				os.RemoveAll(copyDir)
			})

			when("the order.toml exists", func() {
				it("processes the provided order.toml", func() {
					h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						"/layers",
						detectImage,
						h.WithFlags("--user", userID,
							"--volume", expectedOrderTOMLPath+":/custom/order.toml",
							"--env", "CNB_PLATFORM_API="+platformAPI,
						),
						h.WithArgs(
							"-log-level=debug",
							"-order=/custom/order.toml",
						),
					)

					// check group.toml
					foundGroupTOML := filepath.Join(copyDir, "layers", "group.toml")
					group, err := files.Handler.ReadGroup(foundGroupTOML)
					h.AssertNil(t, err)
					h.AssertEq(t, group.Group[0].ID, "simple_buildpack")
					h.AssertEq(t, group.Group[0].Version, "simple_buildpack_version")
				})
			})

			when("the order.toml does not exist", func() {
				it("errors", func() {
					command := exec.Command("docker", "run",
						"--user", userID,
						"--rm",
						"--env", "CNB_PLATFORM_API="+platformAPI,
						detectImage,
						"-order=/custom/order.toml")
					output, err := command.CombinedOutput()
					h.AssertNotNil(t, err)
					expected := "failed to initialize detector: reading order: failed to read order file: open /custom/order.toml: no such file or directory"
					h.AssertStringContains(t, string(output), expected)
				})
			})

			when("the order.toml contains a buildpack using an unsupported api", func() {
				it("errors", func() {
					command := exec.Command("docker", "run",
						"--user", userID,
						"--rm",
						"--env", "CNB_PLATFORM_API="+platformAPI,
						detectImage,
						"-order=/cnb/orders/bad_api.toml")
					output, err := command.CombinedOutput()
					h.AssertNotNil(t, err)
					failErr, ok := err.(*exec.ExitError)
					if !ok {
						t.Fatalf("expected an error of type exec.ExitError")
					}
					h.AssertEq(t, failErr.ExitCode(), 12) // platform code for buildpack api error
					expected := "buildpack API version '0.1' is incompatible with the lifecycle"
					h.AssertStringContains(t, string(output), expected)
				})
			})
		})

		when("-order contains extensions", func() {
			var containerName, copyDir, orderPath string

			it.Before(func() {
				containerName = "test-container-" + h.RandString(10)
				var err error
				copyDir, err = os.MkdirTemp("", "test-docker-copy-")
				h.AssertNil(t, err)
				orderPath, err = filepath.Abs(filepath.Join("testdata", "detector", "container", "cnb", "orders", "order_with_ext.toml"))
				h.AssertNil(t, err)
			})

			it.After(func() {
				if h.DockerContainerExists(t, containerName) {
					h.Run(t, exec.Command("docker", "rm", containerName))
				}
				os.RemoveAll(copyDir)
			})

			it("processes the provided order.toml", func() {
				experimentalMode := "warn"
				if api.MustParse(platformAPI).AtLeast("0.13") {
					experimentalMode = "error"
				}

				output := h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					"/layers",
					detectImage,
					h.WithFlags(
						"--user", userID,
						"--volume", orderPath+":/layers/order.toml",
						"--env", "CNB_PLATFORM_API="+platformAPI,
						"--env", "CNB_EXPERIMENTAL_MODE="+experimentalMode,
					),
					h.WithArgs(
						"-analyzed=/layers/analyzed.toml",
						"-extensions=/cnb/extensions",
						"-generated=/layers/generated",
						"-log-level=debug",
						"-run=/layers/run.toml", // /cnb/run.toml is the default location of run.toml
					),
				)

				t.Log("runs /bin/detect for buildpacks and extensions")
				if api.MustParse(platformAPI).LessThan("0.13") {
					h.AssertStringContains(t, output, "Platform requested experimental feature 'Dockerfiles'")
				}
				h.AssertStringContains(t, output, "FOO=val-from-build-config")
				h.AssertStringContains(t, output, "simple_extension: output from /bin/detect")
				t.Log("writes group.toml")
				foundGroupTOML := filepath.Join(copyDir, "layers", "group.toml")
				group, err := files.Handler.ReadGroup(foundGroupTOML)
				h.AssertNil(t, err)
				h.AssertEq(t, group.GroupExtensions[0].ID, "simple_extension")
				h.AssertEq(t, group.GroupExtensions[0].Version, "simple_extension_version")
				h.AssertEq(t, group.Group[0].ID, "buildpack_for_ext")
				h.AssertEq(t, group.Group[0].Version, "buildpack_for_ext_version")
				h.AssertEq(t, group.Group[0].Extension, false)
				t.Log("writes plan.toml")
				foundPlanTOML := filepath.Join(copyDir, "layers", "plan.toml")
				buildPlan, err := files.Handler.ReadPlan(foundPlanTOML)
				h.AssertNil(t, err)
				h.AssertEq(t, len(buildPlan.Entries), 0) // this shows that the plan was filtered to remove `requires` provided by extensions

				t.Log("runs /bin/generate for extensions")
				h.AssertStringContains(t, output, "simple_extension: output from /bin/generate")

				var dockerfilePath string
				if api.MustParse(platformAPI).LessThan("0.13") {
					t.Log("copies the generated Dockerfiles to the output directory")
					dockerfilePath = filepath.Join(copyDir, "layers", "generated", "run", "simple_extension", "Dockerfile")
				} else {
					dockerfilePath = filepath.Join(copyDir, "layers", "generated", "simple_extension", "run.Dockerfile")
				}
				h.AssertPathExists(t, dockerfilePath)
				contents, err := os.ReadFile(dockerfilePath)
				h.AssertEq(t, string(contents), "FROM some-run-image-from-extension\n")
				t.Log("records the new run image in analyzed.toml")
				foundAnalyzedTOML := filepath.Join(copyDir, "layers", "analyzed.toml")
				analyzedMD, err := files.Handler.ReadAnalyzed(foundAnalyzedTOML, cmd.DefaultLogger)
				h.AssertNil(t, err)
				h.AssertEq(t, analyzedMD.RunImage.Image, "some-run-image-from-extension")
			})
		})

		when("Platform API >= 0.15", func() {
			when("system.toml is provided", func() {
				var copyDir, containerName string

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
					os.RemoveAll(copyDir)
				})

				it("merges system buildpacks with order", func() {
					if api.MustParse(platformAPI).LessThan("0.15") {
						t.Skip("skipping test for Platform API < 0.15")
					}
					output := h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						"/layers",
						detectImage,
						h.WithFlags("--user", userID,
							"--env", "CNB_ORDER_PATH=/cnb/orders/middle_order.toml",
							"--env", "CNB_SYSTEM_PATH=/cnb/system.toml",
							"--env", "CNB_PLATFORM_API="+platformAPI,
						),
						h.WithArgs("-log-level=debug"),
					)

					t.Log("system buildpacks are prepended and appended to the order")
					// Expected order after merging:
					// 1. always_detect_buildpack (pre)
					// 2. buildpack_for_ext (from middle_order.toml)
					// 3. simple_buildpack (post)

					// check group.toml - should contain all three buildpacks
					foundGroupTOML := filepath.Join(copyDir, "layers", "group.toml")
					group, err := files.Handler.ReadGroup(foundGroupTOML)
					h.AssertNil(t, err)

					h.AssertEq(t, len(group.Group), 3)
					h.AssertEq(t, group.Group[0].ID, "always_detect_buildpack")
					h.AssertEq(t, group.Group[0].Version, "always_detect_buildpack_version")
					h.AssertEq(t, group.Group[1].ID, "buildpack_for_ext")
					h.AssertEq(t, group.Group[1].Version, "buildpack_for_ext_version")
					h.AssertEq(t, group.Group[2].ID, "simple_buildpack")
					h.AssertEq(t, group.Group[2].Version, "simple_buildpack_version")

					// check output contains all buildpacks
					h.AssertStringContains(t, output, "always_detect_buildpack always_detect_buildpack_version")
					h.AssertStringContains(t, output, "buildpack_for_ext buildpack_for_ext_version")
					h.AssertStringContains(t, output, "simple_buildpack simple_buildpack_version")

					// check debug logs show system buildpack merging
					h.AssertStringContains(t, output, "Prepending 1 system buildpack(s) to order")
					h.AssertStringContains(t, output, "Appending 1 system buildpack(s) to order")
				})
			})
		})
	}
}
