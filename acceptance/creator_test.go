//go:build acceptance
// +build acceptance

package acceptance

import (
	"io/ioutil"
	"math/rand"
	"os"
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
	createImage          string
	createRegAuthConfig  string
	createRegNetwork     string
	creatorPath          string
	createDaemonFixtures *daemonImageFixtures
	createRegFixtures    *regImageFixtures
	createTest           *PhaseTest
)

func TestCreator(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "Creator acceptance tests are not yet supported on Windows")

	rand.Seed(time.Now().UTC().UnixNano())

	testImageDockerContext := filepath.Join("testdata", "creator")
	createTest = NewPhaseTest(t, "creator", testImageDockerContext)
	createTest.Start(t)
	defer createTest.Stop(t)

	createImage = createTest.testImageRef
	creatorPath = createTest.containerBinaryPath
	cacheFixtureDir = filepath.Join("testdata", "creator", "cache-dir")
	createRegAuthConfig = createTest.targetRegistry.authConfig
	createRegNetwork = createTest.targetRegistry.network
	createDaemonFixtures = createTest.targetDaemon.fixtures
	createRegFixtures = createTest.targetRegistry.fixtures

	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "acceptance-creator/"+platformAPI.String(), testCreatorFunc(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testCreatorFunc(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var createdImageName string

		when("daemon case", func() {
			it.After(func() {
				h.DockerImageRemove(t, createdImageName)
			})

			when("first build", func() {
				when("app", func() {
					it("is created", func() {
						createFlags := []string{"-daemon"}
						createFlags = append(createFlags, []string{"-run-image", createRegFixtures.ReadOnlyRunImage}...)

						createArgs := append([]string{ctrPath(creatorPath)}, createFlags...)
						createdImageName = "some-created-image-" + h.RandString(10)
						createArgs = append(createArgs, createdImageName)

						output := h.DockerRun(t,
							createImage,
							h.WithFlags(append(
								dockerSocketMount,
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_REGISTRY_AUTH="+createRegAuthConfig,
								"--network", createRegNetwork,
							)...),
							h.WithArgs(createArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+createdImageName)

						assertImageOSAndArch(t, createdImageName, createTest)

						output = h.DockerRun(t,
							createdImageName,
							h.WithFlags(
								"--entrypoint", "/cnb/lifecycle/launcher",
							),
							h.WithArgs("env"),
						)
						h.AssertStringContains(t, output, "SOME_VAR=some-val") // set by buildpack
					})
				})
			})
		})

		when("registry case", func() {
			it.After(func() {
				h.DockerImageRemove(t, createdImageName)
			})

			when("first build", func() {
				when("app", func() {
					it("is created", func() {
						var createFlags []string
						createFlags = append(createFlags, []string{"-run-image", createRegFixtures.ReadOnlyRunImage}...)

						createArgs := append([]string{ctrPath(creatorPath)}, createFlags...)
						createdImageName = createTest.RegRepoName("some-created-image-" + h.RandString(10))
						createArgs = append(createArgs, createdImageName)

						output := h.DockerRun(t,
							createImage,
							h.WithFlags(
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_REGISTRY_AUTH="+createRegAuthConfig,
								"--network", createRegNetwork,
							),
							h.WithArgs(createArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+createdImageName)

						h.Run(t, exec.Command("docker", "pull", createdImageName))
						assertImageOSAndArch(t, createdImageName, createTest)

						output = h.DockerRun(t,
							createdImageName,
							h.WithFlags(
								"--entrypoint", "/cnb/lifecycle/launcher",
							),
							h.WithArgs("env"),
						)
						h.AssertStringContains(t, output, "SOME_VAR=some-val") // set by buildpack
					})
				})
			})
		})

		when.Focus("sbom", func() {
			var (
				cacheDir  string
				layersDir string
				secondLayersDir string
			)

			it.Before(func() {
				var err error
				cacheDir, err = ioutil.TempDir("", "creator-acceptance")
				h.AssertNil(t, err)
				layersDir, err = ioutil.TempDir("", "creator-acceptance")
				h.AssertNil(t, err)
				secondLayersDir, err = ioutil.TempDir("", "creator-acceptance")
				h.AssertNil(t, err)
			})

			it.After(func() {
				h.AssertNil(t, os.RemoveAll(cacheDir))
				h.AssertNil(t, os.RemoveAll(layersDir))
				h.AssertNil(t, os.RemoveAll(secondLayersDir))
			})

			it("is exported in the app image", func() {
				h.SkipIf(t, runtime.GOOS == "windows", "Test needs to be adapted to work on Windows")
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.8"), "Platform API < 0.8 does not support standardized sBOM")

				createFlags := []string{"-daemon"}
				createFlags = append(createFlags, []string{
					"-run-image", createRegFixtures.ReadOnlyRunImage,
					"-cache-dir", "/cache",
					"-log-level", "debug",
				}...)

				createArgs := append([]string{ctrPath(creatorPath)}, createFlags...)
				createdImageName = "some-created-image-" + h.RandString(10)
				createArgs = append(createArgs, createdImageName)

				// first build
				output := h.DockerRun(t,
					createImage,
					h.WithFlags(append(
						dockerSocketMount,
						"--env", "CNB_PLATFORM_API="+platformAPI,
						"--env", "CNB_REGISTRY_AUTH="+createRegAuthConfig,
						"--network", createRegNetwork,
						"--volume", cacheDir+":/cache",
						"--volume", layersDir+":/layers",
					)...),
					h.WithArgs(createArgs...),
				)
				h.AssertStringDoesNotContain(t, string(output), "restored with content")
				h.AssertPathExists(t, filepath.Join(layersDir, "sbom", "build", "samples_hello-world", "sbom.cdx.json"))
				h.AssertPathExists(t, filepath.Join(layersDir, "sbom", "build", "samples_hello-world", "some-build-layer", "sbom.cdx.json"))

				// first run
				output = h.DockerRun(t,
					createdImageName,
					h.WithFlags(
						"--entrypoint", "/bin/bash",
					),
					h.WithArgs("-c", "ls -Rl /layers && "+
						"ls /layers/sbom/launch/samples_hello-world/sbom.cdx.json && "+
						"ls /layers/sbom/launch/samples_hello-world/some-launch-cache-layer/sbom.cdx.json && "+
						"ls /layers/sbom/launch/samples_hello-world/some-layer/sbom.cdx.json"),
				)
				h.AssertStringDoesNotContain(t, string(output), "/layers/sbom/build")
				h.AssertStringDoesNotContain(t, string(output), "some-build-layer")
				h.AssertStringDoesNotContain(t, string(output), "some-cache-layer")

				// second build
				output = h.DockerRun(t,
					createImage,
					h.WithFlags(append(
						dockerSocketMount,
						"--env", "CNB_PLATFORM_API="+platformAPI,
						"--env", "CNB_REGISTRY_AUTH="+createRegAuthConfig,
						"--network", createRegNetwork,
						"--volume", cacheDir+":/cache",
						"--volume", secondLayersDir+":/layers",
					)...),
					h.WithArgs(createArgs...),
				)
				h.AssertStringContains(t, string(output), "some-layer.sbom.cdx.json restored with content: {\"key\": \"some-launch-true-bom-content\"}")
				h.AssertStringContains(t, string(output), "some-cache-layer.sbom.cdx.json restored with content: {\"key\": \"some-cache-true-bom-content\"}")
				h.AssertStringContains(t, string(output), "some-launch-cache-layer.sbom.cdx.json restored with content: {\"key\": \"some-launch-true-cache-true-bom-content\"}")
				h.AssertStringContains(t, string(output), "Reusing layer 'launch.sbom'")
				h.AssertPathExists(t, filepath.Join(secondLayersDir, "sbom", "build", "samples_hello-world", "sbom.cdx.json"))
				h.AssertPathExists(t, filepath.Join(secondLayersDir, "sbom", "build", "samples_hello-world", "some-build-layer", "sbom.cdx.json"))

				// second run
				output = h.DockerRun(t,
					createdImageName,
					h.WithFlags(
						"--entrypoint", "/bin/bash",
					),
					h.WithArgs("-c", "ls -Rl /layers && "+
						"ls /layers/sbom/launch/samples_hello-world/sbom.cdx.json && "+
						"ls /layers/sbom/launch/samples_hello-world/some-launch-cache-layer/sbom.cdx.json && "+
						"ls /layers/sbom/launch/samples_hello-world/some-layer/sbom.cdx.json"),
				)
				h.AssertStringDoesNotContain(t, string(output), "/layers/sbom/build")
				h.AssertStringDoesNotContain(t, string(output), "some-build-layer")
				h.AssertStringDoesNotContain(t, string(output), "some-cache-layer")

				h.DockerImageRemove(t, createdImageName)
			})
		})
	}
}
