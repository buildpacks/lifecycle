//go:build acceptance
// +build acceptance

package acceptance

import (
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/buildpacks/lifecycle/internal/path"

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

		when("called with run", func() {
			it("uses the provided run.toml path", func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "Platform API < 0.12 does not accept -run")
				cmd := exec.Command(
					"docker", "run", "--rm",
					"--env", "CNB_PLATFORM_API="+platformAPI,
					"--env", "CNB_REGISTRY_AUTH="+createRegAuthConfig,
					"--network", createRegNetwork,
					createImage,
					ctrPath(creatorPath),
					"-run", "/cnb/run.toml",
					createRegFixtures.SomeAppImage,
				) // #nosec G204
				output, err := cmd.CombinedOutput()

				h.AssertNotNil(t, err)
				expected := "ensure registry read access to some-run-image-from-run-toml"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("daemon case", func() {
			it.After(func() {
				h.DockerImageRemove(t, createdImageName)
			})

			it("creates app", func() {
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

		when("registry case", func() {
			it.After(func() {
				h.DockerImageRemove(t, createdImageName)
			})

			it("creates app", func() {
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

		when("multiple builds", func() {
			var (
				container1     string
				container2     string
				container3     string
				container4     string
				dirBuild1      string
				dirBuild2      string
				dirCache       string
				dirLaunchCache string
				dirRun1        string
				dirRun2        string
				imageName      string
			)

			it.Before(func() {
				// assign container names
				for _, cPtr := range []*string{&container1, &container2, &container3, &container4} {
					*cPtr = "test-container-" + h.RandString(10)
				}
				// create temp dirs
				for _, dirPtr := range []*string{&dirCache, &dirLaunchCache, &dirBuild1, &dirRun1, &dirBuild2, &dirRun2} {
					dir, err := os.MkdirTemp("", "creator-acceptance")
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
				// remove containers if needed
				for _, container := range []string{container1, container2, container3, container4} {
					if h.DockerContainerExists(t, container) {
						h.Run(t, exec.Command("docker", "rm", container))
					}
				}
				// remove temp dirs
				for _, dir := range []string{dirCache, dirLaunchCache, dirBuild1, dirRun1, dirBuild2, dirRun2} {
					_ = os.RemoveAll(dir)
				}
				// remove image
				h.DockerImageRemove(t, imageName)
			})

			when("multiple builds", func() {
				var (
					createFlags          []string
					createArgs           []string
					duration1, duration2 time.Duration
				)

				it.Before(func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.8"), "Platform API < 0.8 does not support standardized SBOM")

					createFlags = []string{"-daemon"}
					createFlags = append(createFlags, []string{
						"-run-image", createRegFixtures.ReadOnlyRunImage,
						"-cache-dir", ctrPath("/cache"),
						"-launch-cache", ctrPath("/launch-cache"),
						"-log-level", "debug",
					}...)
					createArgs = append([]string{ctrPath(creatorPath)}, createFlags...)
					createArgs = append(createArgs, imageName)

					startTime := time.Now()
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
							"--volume", dirLaunchCache+":"+ctrPath("/launch-cache"),
						)...),
						h.WithArgs(createArgs...),
					)
					duration1 = time.Now().Sub(startTime)
					t.Logf("First build duration: %s", duration1)
					h.AssertStringDoesNotContain(t, output, "restored with content")
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
				})

				when("rebuild with cache", func() {
					it("exports SBOM in the app image", func() {
						startTime := time.Now()
						// second build
						output := h.DockerRunAndCopy(t,
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
								"--volume", dirLaunchCache+":"+ctrPath("/launch-cache"),
							)...),
							h.WithArgs(createArgs...),
						)
						// check that launch cache was used
						duration2 = time.Now().Sub(startTime)
						t.Logf("Second build duration: %s", duration2)
						if duration2+time.Duration(0.1*float64(time.Second)) >= duration1 {
							t.Logf("Second build output: %s", output)
							t.Fatalf("Expected second build to complete 0.1s faster than first build; first build took %s, second build took %s", duration1, duration2)
						}
						h.AssertStringContains(t, output, "some-layer.sbom.cdx.json restored with content: {\"key\": \"some-launch-true-bom-content\"}")
						h.AssertStringContains(t, output, "some-cache-layer.sbom.cdx.json restored with content: {\"key\": \"some-cache-true-bom-content\"}")
						h.AssertStringContains(t, output, "some-launch-cache-layer.sbom.cdx.json restored with content: {\"key\": \"some-launch-true-cache-true-bom-content\"}")
						h.AssertStringContains(t, output, "Reusing layer 'buildpacksio/lifecycle:launch.sbom'")
						h.AssertPathExists(t, filepath.Join(dirBuild2, "layers", "sbom", "build", "samples_hello-world", "sbom.cdx.json"))
						h.AssertPathExists(t, filepath.Join(dirBuild2, "layers", "sbom", "build", "samples_hello-world", "some-build-layer", "sbom.cdx.json"))
						t.Log("restores store.toml")
						h.AssertStringContains(t, output, "store.toml restored with content: [metadata]")

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

				when("rebuild with clear cache", func() {
					it("exports SBOM in the app image", func() {
						createArgs = append([]string{ctrPath(creatorPath)}, append(createFlags, "-skip-restore")...)
						createArgs = append(createArgs, imageName)

						// second build
						output := h.DockerRunAndCopy(t,
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
								"--volume", dirLaunchCache+":"+ctrPath("/launch-cache"),
							)...),
							h.WithArgs(createArgs...),
						)
						h.AssertStringDoesNotContain(t, output, "some-layer.sbom.cdx.json restored with content: {\"key\": \"some-launch-true-bom-content\"}")
						h.AssertStringDoesNotContain(t, output, "some-cache-layer.sbom.cdx.json restored with content: {\"key\": \"some-cache-true-bom-content\"}")
						h.AssertStringDoesNotContain(t, output, "some-launch-cache-layer.sbom.cdx.json restored with content: {\"key\": \"some-launch-true-cache-true-bom-content\"}")
						// check that store.toml was restored
						if api.MustParse(platformAPI).AtLeast("0.10") {
							h.AssertStringContains(t, output, "store.toml restored with content: [metadata]")
						} else {
							h.AssertStringDoesNotContain(t, output, "store.toml restored with content")
						}
					})
				})
			})
		})

		when("layout case", func() {
			var (
				containerName string
				err           error
				layoutDir     string
				tmpDir        string
			)
			when("experimental mode is enabled", func() {
				it.Before(func() {
					// creates the directory to save all the OCI images on disk
					tmpDir, err = os.MkdirTemp("", "layout")
					h.AssertNil(t, err)

					containerName = "test-container-" + h.RandString(10)
					layoutDir = filepath.Join(path.RootDir, "layout-repo")
				})

				it.After(func() {
					if h.DockerContainerExists(t, containerName) {
						h.Run(t, exec.Command("docker", "rm", containerName))
					}
					h.DockerImageRemove(t, createdImageName)

					// removes all images created
					os.RemoveAll(tmpDir)

				})

				it("creates app", func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "Platform API < 0.12 does not accept a -layout flag")
					var createFlags []string
					createFlags = append(createFlags, []string{"-layout", "-run-image", "busybox"}...)

					createArgs := append([]string{ctrPath(creatorPath)}, createFlags...)
					createdImageName = "some-created-image-" + h.RandString(10)
					createArgs = append(createArgs, createdImageName)

					output := h.DockerRunAndCopy(t, containerName, tmpDir, layoutDir, createImage,
						h.WithFlags(
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--env", "CNB_EXPERIMENTAL_MODE=warn",
						),
						h.WithArgs(createArgs...))

					h.AssertStringContains(t, output, "Saving /layout-repo/index.docker.io/library/"+createdImageName+"/latest")
					index := h.ReadIndexManifest(t, filepath.Join(tmpDir, layoutDir, "index.docker.io", "library", createdImageName+"/latest"))
					h.AssertEq(t, len(index.Manifests), 1)
				})
			})

			when("experimental mode is not enabled", func() {
				it("error message", func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "Platform API < 0.12 does not accept a -layout flag")

					cmd := exec.Command(
						"docker", "run", "--rm",
						"--env", "CNB_PLATFORM_API="+platformAPI,
						createImage,
						ctrPath(creatorPath),
						"-layout",
						"-run-image", "busybox",
						"some-image",
					) // #nosec G204
					output, err := cmd.CombinedOutput()

					h.AssertNotNil(t, err)
					expected := "experimental features are disabled by CNB_EXPERIMENTAL_MODE=error"
					h.AssertStringContains(t, string(output), expected)
				})
			})
		})
	}
}
