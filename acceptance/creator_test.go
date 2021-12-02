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

		when("sbom", func() {
			var (
				container1 string
				container2 string
				container3 string
				container4 string
				dirBuild1  string
				dirBuild2  string
				dirCache   string
				dirRun1    string
				dirRun2    string
				imageName  string
			)

			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.8"), "Platform API < 0.8 does not support standardized sBOM")

				// assign container names
				for _, cPtr := range []*string{&container1, &container2, &container3, &container4} {
					*cPtr = "test-container-" + h.RandString(10)
				}
				// create temp dirs
				for _, dirPtr := range []*string{&dirCache, &dirBuild1, &dirRun1, &dirBuild2, &dirRun2} {
					dir, err := ioutil.TempDir("", "creator-acceptance")
					h.AssertNil(t, err)
					h.AssertNil(t, os.Chmod(dir, 0777)) // Override umask

					// Resolve temp dir so it can be properly mounted by the Docker daemon.
					*dirPtr, err = filepath.EvalSymlinks(dir)
					h.AssertNil(t, err)
				}
				// assign image name
				imageName = "some-created-image-" + h.RandString(10)
			})

			it.After(func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.8"), "Platform API < 0.8 does not support standardized sBOM")

				// remove containers if needed
				for _, container := range []string{container1, container2, container3, container4} {
					if h.DockerContainerExists(t, container) {
						h.Run(t, exec.Command("docker", "rm", container))
					}
				}
				// remove temp dirs
				for _, dir := range []string{dirCache, dirBuild1, dirRun1, dirBuild2, dirRun2} {
					h.AssertNil(t, os.RemoveAll(dir))
				}
				// remove image
				h.DockerImageRemove(t, imageName)
			})

			it("is exported in the app image", func() {
				createFlags := []string{"-daemon"}
				createFlags = append(createFlags, []string{
					"-run-image", createRegFixtures.ReadOnlyRunImage,
					"-cache-dir", ctrPath("/cache"),
					"-log-level", "debug",
				}...)
				createArgs := append([]string{ctrPath(creatorPath)}, createFlags...)
				createArgs = append(createArgs, imageName)

				// first build
				output := h.DockerRunAndCopy(t,
					container1,
					dirBuild1,
					ctrPath("/layers"),
					createImage,
					h.WithFlags(append(
						dockerSocketMount,
						"--env", "CNB_PLATFORM_API="+platformAPI,
						"--env", "CNB_REGISTRY_AUTH="+createRegAuthConfig,
						"--network", createRegNetwork,
						"--volume", dirCache+":"+ctrPath("/cache"),
					)...),
					h.WithArgs(createArgs...),
				)
				h.AssertStringDoesNotContain(t, string(output), "restored with content")
				h.AssertPathExists(t, filepath.Join(dirBuild1, "layers", "sbom", "build", "samples_hello-world", "sbom.cdx.json"))
				h.AssertPathExists(t, filepath.Join(dirBuild1, "layers", "sbom", "build", "samples_hello-world", "some-build-layer", "sbom.cdx.json"))

				// first run
				output = h.DockerRunAndCopy(t,
					container2,
					dirRun1,
					ctrPath("/layers"),
					imageName,
					h.WithFlags(
						"--entrypoint", "/cnb/lifecycle/launcher",
					),
					h.WithArgs("env"),
				)
				h.AssertPathExists(t, filepath.Join(dirRun1, "layers", "sbom", "launch", "samples_hello-world", "sbom.cdx.json"))
				h.AssertPathExists(t, filepath.Join(dirRun1, "layers", "sbom", "launch", "samples_hello-world", "some-launch-cache-layer", "sbom.cdx.json"))
				h.AssertPathExists(t, filepath.Join(dirRun1, "layers", "sbom", "launch", "samples_hello-world", "some-layer", "sbom.cdx.json"))
				h.AssertPathDoesNotExist(t, filepath.Join(dirRun1, "layers", "sbom", "build"))
				h.AssertPathDoesNotExist(t, filepath.Join(dirRun1, "layers", "sbom", "cache"))

				// second build
				output = h.DockerRunAndCopy(t,
					container3,
					dirBuild2,
					ctrPath("/layers"),
					createImage,
					h.WithFlags(append(
						dockerSocketMount,
						"--env", "CNB_PLATFORM_API="+platformAPI,
						"--env", "CNB_REGISTRY_AUTH="+createRegAuthConfig,
						"--network", createRegNetwork,
						"--volume", dirCache+":/cache",
					)...),
					h.WithArgs(createArgs...),
				)
				h.AssertStringContains(t, string(output), "some-layer.sbom.cdx.json restored with content: {\"key\": \"some-launch-true-bom-content\"}")
				h.AssertStringContains(t, string(output), "some-cache-layer.sbom.cdx.json restored with content: {\"key\": \"some-cache-true-bom-content\"}")
				h.AssertStringContains(t, string(output), "some-launch-cache-layer.sbom.cdx.json restored with content: {\"key\": \"some-launch-true-cache-true-bom-content\"}")
				h.AssertStringContains(t, string(output), "Reusing layer 'launch.sbom'")
				h.AssertPathExists(t, filepath.Join(dirBuild2, "layers", "sbom", "build", "samples_hello-world", "sbom.cdx.json"))
				h.AssertPathExists(t, filepath.Join(dirBuild2, "layers", "sbom", "build", "samples_hello-world", "some-build-layer", "sbom.cdx.json"))

				// second run
				output = h.DockerRunAndCopy(t,
					container4,
					dirRun2,
					ctrPath("/layers"),
					imageName,
					h.WithFlags(
						"--entrypoint", "/cnb/lifecycle/launcher",
					),
					h.WithArgs("env"),
				)
				h.AssertPathExists(t, filepath.Join(dirRun1, "layers", "sbom", "launch", "samples_hello-world", "sbom.cdx.json"))
				h.AssertPathExists(t, filepath.Join(dirRun1, "layers", "sbom", "launch", "samples_hello-world", "some-launch-cache-layer", "sbom.cdx.json"))
				h.AssertPathExists(t, filepath.Join(dirRun1, "layers", "sbom", "launch", "samples_hello-world", "some-layer", "sbom.cdx.json"))
				h.AssertPathDoesNotExist(t, filepath.Join(dirRun1, "layers", "sbom", "build"))
				h.AssertPathDoesNotExist(t, filepath.Join(dirRun1, "layers", "sbom", "cache"))
			})
		})
	}
}
