//go:build acceptance
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
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	detectDockerContext = filepath.Join("testdata", "detector")
	detectorBinaryDir   = filepath.Join("testdata", "detector", "container", "cnb", "lifecycle")
	detectImage         = "lifecycle/acceptance/detector"
	userID              = "1234"
)

func TestDetector(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "Detector acceptance tests are not yet supported on Windows")
	h.SkipIf(t, runtime.GOARCH != "amd64", "Detector acceptance tests are not yet supported on non-amd64")

	rand.Seed(time.Now().UTC().UnixNano())

	h.MakeAndCopyLifecycle(t, "linux", "amd64", detectorBinaryDir)
	h.DockerBuild(t,
		detectImage,
		detectDockerContext,
		h.WithArgs("--build-arg", fmt.Sprintf("cnb_platform_api=%s", api.Platform.Latest())),
	)
	defer h.DockerImageRemove(t, detectImage)

	spec.Run(t, "acceptance-detector", testDetector, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testDetector(t *testing.T, when spec.G, it spec.S) {
	when("called with arguments", func() {
		it("errors", func() {
			command := exec.Command(
				"docker",
				"run",
				"--rm",
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				detectImage,
			)
			output, err := command.CombinedOutput()
			h.AssertNotNil(t, err)
			expected := "failed to initialize detector: reading buildpack order file"
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("no buildpack group passed detection", func() {
		it("errors", func() {
			command := exec.Command(
				"docker",
				"run",
				"--rm",
				"--env", "CNB_ORDER_PATH=/cnb/orders/empty_order.toml",
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				detectImage,
			)
			output, err := command.CombinedOutput()
			h.AssertNotNil(t, err)
			failErr, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("expected an error of type exec.ExitError")
			}
			h.AssertEq(t, failErr.ExitCode(), 20) // platform code for cmd.FailedDetect
			expected := "No buildpack groups passed detection."
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("there is a buildpack group that pass detection", func() {
		var copyDir, containerName string

		it.Before(func() {
			containerName = "test-container-" + h.RandString(10)
			var err error
			copyDir, err = ioutil.TempDir("", "test-docker-copy-")
			h.AssertNil(t, err)
		})

		it.After(func() {
			if h.DockerContainerExists(t, containerName) {
				h.Run(t, exec.Command("docker", "rm", containerName))
			}
			os.RemoveAll(copyDir)
		})

		it("writes group.toml and plan.toml", func() {
			h.DockerRunAndCopy(t,
				containerName,
				copyDir,
				"/layers",
				detectImage,
				h.WithFlags("--user", userID,
					"--env", "CNB_ORDER_PATH=/cnb/orders/simple_order.toml",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				),
				h.WithArgs(),
			)

			// check group.toml
			foundGroupTOML := filepath.Join(copyDir, "layers", "group.toml")
			var buildpackGroup buildpack.Group
			_, err := toml.DecodeFile(foundGroupTOML, &buildpackGroup)
			h.AssertNil(t, err)
			h.AssertEq(t, buildpackGroup.Group[0].ID, "simple_buildpack")
			h.AssertEq(t, buildpackGroup.Group[0].Version, "simple_buildpack_version")

			// check plan.toml
			tempPlanToml := filepath.Join(copyDir, "layers", "plan.toml")
			var buildPlan platform.BuildPlan
			_, err = toml.DecodeFile(tempPlanToml, &buildPlan)
			h.AssertNil(t, err)
			h.AssertEq(t, buildPlan.Entries[0].Providers[0].ID, "simple_buildpack")
			h.AssertEq(t, buildPlan.Entries[0].Providers[0].Version, "simple_buildpack_version")
			h.AssertEq(t, buildPlan.Entries[0].Requires[0].Name, "some_requirement")
			h.AssertEq(t, buildPlan.Entries[0].Requires[0].Metadata["some_metadata_key"], "some_metadata_val")
			h.AssertEq(t, buildPlan.Entries[0].Requires[0].Metadata["version"], "some_version")
		})
	})

	when("environment variables are provided for buildpack and app directories and for the output files", func() {
		var copyDir, containerName string

		it.Before(func() {
			containerName = "test-container-" + h.RandString(10)
			var err error
			copyDir, err = ioutil.TempDir("", "test-docker-copy-")
			h.AssertNil(t, err)
		})

		it.After(func() {
			if h.DockerContainerExists(t, containerName) {
				h.Run(t, exec.Command("docker", "rm", containerName))
			}
			os.RemoveAll(copyDir)
		})

		it("writes group.toml and plan.toml in the right location and with the right names", func() {
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
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				),
				h.WithArgs("-log-level=debug"),
			)

			// check group.toml
			foundGroupTOML := filepath.Join(copyDir, "layers", "custom_group.toml")
			var buildpackGroup buildpack.Group
			_, err := toml.DecodeFile(foundGroupTOML, &buildpackGroup)
			h.AssertNil(t, err)
			h.AssertEq(t, buildpackGroup.Group[0].ID, "always_detect_buildpack")
			h.AssertEq(t, buildpackGroup.Group[0].Version, "always_detect_buildpack_version")

			// check plan.toml - should be empty since we're using always_detect_order.toml so there is no "actual plan"
			tempPlanToml := filepath.Join(copyDir, "layers", "custom_plan.toml")
			planContents, err := ioutil.ReadFile(tempPlanToml)
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
			copyDir, err = ioutil.TempDir("", "test-docker-copy-")
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
						"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					),
					h.WithArgs(
						"-log-level=debug",
						"-order=/custom/order.toml",
					),
				)

				// check group.toml
				foundGroupTOML := filepath.Join(copyDir, "layers", "group.toml")
				var buildpackGroup buildpack.Group
				_, err := toml.DecodeFile(foundGroupTOML, &buildpackGroup)
				h.AssertNil(t, err)
				h.AssertEq(t, buildpackGroup.Group[0].ID, "simple_buildpack")
				h.AssertEq(t, buildpackGroup.Group[0].Version, "simple_buildpack_version")
			})
		})

		when("the order.toml does not exist", func() {
			it("errors", func() {
				command := exec.Command("docker", "run",
					"--user", userID,
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					detectImage,
					"-order=/custom/order.toml")
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "failed to initialize detector: reading buildpack order file: open /custom/order.toml: no such file or directory"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("the order.toml contains a buildpack using an unsupported api", func() {
			it("errors", func() {
				command := exec.Command("docker", "run",
					"--user", userID,
					"--rm",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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

	when("-order is not provided", func() {
		var copyDir, containerName, expectedOrderTOMLPath, otherOrderTOMLPath string

		it.Before(func() {
			containerName = "test-container-" + h.RandString(10)
			var err error
			copyDir, err = ioutil.TempDir("", "test-docker-copy-")
			h.AssertNil(t, err)

			simpleOrderTOML := filepath.Join("testdata", "detector", "container", "cnb", "orders", "simple_order.toml")
			expectedOrderTOMLPath, err = filepath.Abs(simpleOrderTOML)
			h.AssertNil(t, err)

			alwaysDetectOrderTOML := filepath.Join("testdata", "detector", "container", "cnb", "orders", "always_detect_order.toml")
			otherOrderTOMLPath, err = filepath.Abs(alwaysDetectOrderTOML)
			h.AssertNil(t, err)
		})

		it.After(func() {
			if h.DockerContainerExists(t, containerName) {
				h.Run(t, exec.Command("docker", "rm", containerName))
			}
			os.RemoveAll(copyDir)
		})

		when("/cnb/order.toml and /layers/order.toml are present", func() {
			it("prefers /layers/order.toml", func() {
				h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					"/layers",
					detectImage,
					h.WithFlags("--user", userID,
						"--volume", expectedOrderTOMLPath+":/layers/order.toml",
						"--volume", otherOrderTOMLPath+":/cnb/order.toml",
						"--env", "CNB_ORDER_PATH=",
						"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					),
					h.WithArgs("-log-level=debug"),
				)

				// check group.toml
				foundGroupTOML := filepath.Join(copyDir, "layers", "group.toml")
				var buildpackGroup buildpack.Group
				_, err := toml.DecodeFile(foundGroupTOML, &buildpackGroup)
				h.AssertNil(t, err)
				h.AssertEq(t, buildpackGroup.Group[0].ID, "simple_buildpack")
				h.AssertEq(t, buildpackGroup.Group[0].Version, "simple_buildpack_version")
			})
		})

		when("only /cnb/order.toml is present", func() {
			it("processes /cnb/order.toml", func() {
				h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					"/layers",
					detectImage,
					h.WithFlags("--user", userID,
						"--volume", expectedOrderTOMLPath+":/cnb/order.toml",
						"--env", "CNB_ORDER_PATH=",
						"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					),
					h.WithArgs("-log-level=debug"),
				)

				// check group.toml
				foundGroupTOML := filepath.Join(copyDir, "layers", "group.toml")
				var buildpackGroup buildpack.Group
				_, err := toml.DecodeFile(foundGroupTOML, &buildpackGroup)
				h.AssertNil(t, err)
				h.AssertEq(t, buildpackGroup.Group[0].ID, "simple_buildpack")
				h.AssertEq(t, buildpackGroup.Group[0].Version, "simple_buildpack_version")
			})
		})

		when("only /layers/order.toml is present", func() {
			it("processes /layers/order.toml", func() {
				h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					"/layers",
					detectImage,
					h.WithFlags("--user", userID,
						"--volume", expectedOrderTOMLPath+":/layers/order.toml",
						"--env", "CNB_ORDER_PATH=",
						"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
					),
					h.WithArgs("-log-level=debug"),
				)

				// check group.toml
				foundGroupTOML := filepath.Join(copyDir, "layers", "group.toml")
				var buildpackGroup buildpack.Group
				_, err := toml.DecodeFile(foundGroupTOML, &buildpackGroup)
				h.AssertNil(t, err)
				h.AssertEq(t, buildpackGroup.Group[0].ID, "simple_buildpack")
				h.AssertEq(t, buildpackGroup.Group[0].Version, "simple_buildpack_version")
			})
		})

		when("platform api < 0.6", func() {
			when("/cnb/order.toml and /layers/order.toml are present", func() {
				it("only processes /cnb/order.toml", func() {
					h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						"/layers",
						detectImage,
						h.WithFlags("--user", userID,
							"--volume", expectedOrderTOMLPath+":/cnb/order.toml",
							"--volume", otherOrderTOMLPath+":/layers/order.toml",
							"--env", "CNB_PLATFORM_API=0.5",
							"--env", "CNB_ORDER_PATH=",
						),
						h.WithArgs("-log-level=debug"),
					)

					// check group.toml
					foundGroupTOML := filepath.Join(copyDir, "layers", "group.toml")
					var buildpackGroup buildpack.Group
					_, err := toml.DecodeFile(foundGroupTOML, &buildpackGroup)
					h.AssertNil(t, err)
					h.AssertEq(t, buildpackGroup.Group[0].ID, "simple_buildpack")
					h.AssertEq(t, buildpackGroup.Group[0].Version, "simple_buildpack_version")
				})
			})

			when("only /layers/order.toml is present", func() {
				it("errors", func() {
					command := exec.Command("docker", "run",
						"--user", userID,
						"--volume", otherOrderTOMLPath+":/layers/order.toml",
						"--env", "CNB_PLATFORM_API=0.5",
						"--env", "CNB_ORDER_PATH=",
						"--rm", detectImage)
					output, err := command.CombinedOutput()
					h.AssertNotNil(t, err)
					expected := "failed to initialize detector: reading buildpack order file: open /cnb/order.toml: no such file or directory"
					h.AssertStringContains(t, string(output), expected)
				})
			})
		})
	})

	when("-order contains extensions", func() {
		var containerName, copyDir, orderPath string

		it.Before(func() {
			containerName = "test-container-" + h.RandString(10)
			var err error
			copyDir, err = ioutil.TempDir("", "test-docker-copy-")
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
			h.DockerRunAndCopy(t,
				containerName,
				copyDir,
				"/layers",
				detectImage,
				h.WithFlags(
					"--user", userID,
					"--volume", orderPath+":/layers/order.toml",
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				),
				h.WithArgs(
					"-analyzed=/layers/analyzed.toml",
					"-extensions=/cnb/extensions",
					"-log-level=debug",
					"-output-dir=/layers",
				),
			)

			t.Log("runs /bin/detect for buildpacks and extensions")
			t.Log("writes group.toml")
			foundGroupTOML := filepath.Join(copyDir, "layers", "group.toml")
			var buildpackGroup buildpack.Group
			_, err := toml.DecodeFile(foundGroupTOML, &buildpackGroup)
			h.AssertNil(t, err)
			h.AssertEq(t, buildpackGroup.GroupExtensions[0].ID, "simple_extension")
			h.AssertEq(t, buildpackGroup.GroupExtensions[0].Version, "simple_extension_version")
			h.AssertEq(t, buildpackGroup.GroupExtensions[0].Extension, false) // this shows that `extension = true` is not redundantly printed in group.toml
			h.AssertEq(t, buildpackGroup.Group[0].ID, "buildpack_for_ext")
			h.AssertEq(t, buildpackGroup.Group[0].Version, "buildpack_for_ext_version")
			h.AssertEq(t, buildpackGroup.Group[0].Extension, false)
			t.Log("writes plan.toml")
			foundPlanTOML := filepath.Join(copyDir, "layers", "plan.toml")
			var plan platform.BuildPlan
			_, err = toml.DecodeFile(foundPlanTOML, &plan)
			h.AssertNil(t, err)
			h.AssertEq(t, plan.Entries[0].Requires[0].Name, "some_requirement")
			h.AssertEq(t, plan.Entries[0].Providers[0].ID, "simple_extension")
			h.AssertEq(t, plan.Entries[0].Providers[0].Extension, true)

			t.Log("runs /bin/generate for extensions")
			t.Log("copies the generated dockerfiles to the output directory")
			dockerfilePath := filepath.Join(copyDir, "layers", "generated", "run", "simple_extension", "Dockerfile")
			h.AssertPathExists(t, dockerfilePath)
			contents, err := ioutil.ReadFile(dockerfilePath)
			h.AssertEq(t, string(contents), "FROM some-run-image-from-extension\n")
			t.Log("records the new run image in analyzed.toml")
			foundAnalyzedTOML := filepath.Join(copyDir, "layers", "analyzed.toml")
			var analyzed platform.AnalyzedMetadata
			_, err = toml.DecodeFile(foundAnalyzedTOML, &analyzed)
			h.AssertNil(t, err)
			h.AssertEq(t, analyzed.RunImage.Reference, "some-run-image-from-extension")
		})
	})

	when("platform api < 0.6", func() {
		when("no buildpack group passed detection", func() {
			it("errors", func() {
				command := exec.Command(
					"docker",
					"run",
					"--rm",
					"--env", "CNB_ORDER_PATH=/cnb/orders/empty_order.toml",
					"--env", "CNB_PLATFORM_API=0.5",
					detectImage,
				)
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				failErr, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("expected an error of type exec.ExitError")
				}
				h.AssertEq(t, failErr.ExitCode(), 100) // platform code for cmd.FailedDetect
				expected := "No buildpack groups passed detection."
				h.AssertStringContains(t, string(output), expected)
			})
		})
	})
}
