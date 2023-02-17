//go:build acceptance
// +build acceptance

package acceptance

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/buildpacks/imgutil"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/internal/path"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	exportImage          string
	exportRegAuthConfig  string
	exportRegNetwork     string
	exporterPath         string
	exportDaemonFixtures *daemonImageFixtures
	exportRegFixtures    *regImageFixtures
	exportTest           *PhaseTest
)

func TestExporter(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "Exporter acceptance tests are not yet supported on Windows")

	rand.Seed(time.Now().UTC().UnixNano())

	testImageDockerContext := filepath.Join("testdata", "exporter")
	exportTest = NewPhaseTest(t, "exporter", testImageDockerContext)

	exportTest.Start(t, updateAnalyzedTOMLFixturesWithRegRepoName)
	defer exportTest.Stop(t)

	exportImage = exportTest.testImageRef
	exporterPath = exportTest.containerBinaryPath
	cacheFixtureDir = filepath.Join("testdata", "exporter", "cache-dir")
	exportRegAuthConfig = exportTest.targetRegistry.authConfig
	exportRegNetwork = exportTest.targetRegistry.network
	exportDaemonFixtures = exportTest.targetDaemon.fixtures
	exportRegFixtures = exportTest.targetRegistry.fixtures

	rand.Seed(time.Now().UTC().UnixNano())

	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "acceptance-exporter/"+platformAPI.String(), testExporterFunc(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testExporterFunc(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var exportedImageName string

		it.After(func() {
			_, _, _ = h.RunE(exec.Command("docker", "rmi", exportedImageName)) // #nosec G204
		})

		when("daemon case", func() {
			when("first build", func() {
				when("app", func() {
					it("is created", func() {
						exportFlags := []string{"-daemon", "-log-level", "debug"}
						if api.MustParse(platformAPI).LessThan("0.7") {
							exportFlags = append(exportFlags, []string{"-run-image", exportRegFixtures.ReadOnlyRunImage}...)
						}

						exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
						exportedImageName = "some-exported-image-" + h.RandString(10)
						exportArgs = append(exportArgs, exportedImageName)

						output := h.DockerRun(t,
							exportImage,
							h.WithFlags(append(
								dockerSocketMount,
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_REGISTRY_AUTH="+exportRegAuthConfig,
								"--network", exportRegNetwork,
							)...),
							h.WithArgs(exportArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+exportedImageName)

						if api.MustParse(platformAPI).AtLeast("0.11") {
							extensions := []string{"sbom.cdx.json", "sbom.spdx.json", "sbom.syft.json"}
							for _, extension := range extensions {
								h.AssertStringContains(t, output, fmt.Sprintf("Copying SBOM lifecycle.%s to %s", extension, filepath.Join(path.RootDir, "layers", "sbom", "build", "buildpacksio_lifecycle", extension)))
								h.AssertStringContains(t, output, fmt.Sprintf("Copying SBOM launcher.%s to %s", extension, filepath.Join(path.RootDir, "layers", "sbom", "launch", "buildpacksio_lifecycle", "launcher", extension)))
							}
						} else {
							h.AssertStringDoesNotContain(t, output, "Copying SBOM")
						}

						assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, imgutil.NormalizedDateTime)
					})
				})
			})

			when("SOURCE_DATE_EPOCH is set", func() {
				it("Image CreatedAt is set to SOURCE_DATE_EPOCH", func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.9"), "SOURCE_DATE_EPOCH support added in 0.9")
					expectedTime := time.Date(2022, 1, 5, 5, 5, 5, 0, time.UTC)

					exportFlags := []string{"-daemon"}
					if api.MustParse(platformAPI).LessThan("0.7") {
						exportFlags = append(exportFlags, []string{"-run-image", exportRegFixtures.ReadOnlyRunImage}...)
					}

					exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
					exportedImageName = "some-exported-image-" + h.RandString(10)
					exportArgs = append(exportArgs, exportedImageName)

					output := h.DockerRun(t,
						exportImage,
						h.WithFlags(append(
							dockerSocketMount,
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--env", "CNB_REGISTRY_AUTH="+exportRegAuthConfig,
							"--env", "SOURCE_DATE_EPOCH="+fmt.Sprintf("%d", expectedTime.Unix()),
							"--network", exportRegNetwork,
						)...),
						h.WithArgs(exportArgs...),
					)
					h.AssertStringContains(t, output, "Saving "+exportedImageName)

					assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, expectedTime)
				})
			})
		})

		when("registry case", func() {
			when("first build", func() {
				when("app", func() {
					it("is created", func() {
						var exportFlags []string
						if api.MustParse(platformAPI).LessThan("0.7") {
							exportFlags = append(exportFlags, []string{"-run-image", exportRegFixtures.ReadOnlyRunImage}...)
						}

						exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
						exportedImageName = exportTest.RegRepoName("some-exported-image-" + h.RandString(10))
						exportArgs = append(exportArgs, exportedImageName)

						output := h.DockerRun(t,
							exportImage,
							h.WithFlags(
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_REGISTRY_AUTH="+exportRegAuthConfig,
								"--network", exportRegNetwork,
							),
							h.WithArgs(exportArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+exportedImageName)

						h.Run(t, exec.Command("docker", "pull", exportedImageName))
						assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, imgutil.NormalizedDateTime)
					})
				})
				when("SOURCE_DATE_EPOCH is set", func() {
					it("Image CreatedAt is set to SOURCE_DATE_EPOCH", func() {
						h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.9"), "SOURCE_DATE_EPOCH support added in 0.9")
						expectedTime := time.Date(2022, 1, 5, 5, 5, 5, 0, time.UTC)

						var exportFlags []string
						if api.MustParse(platformAPI).LessThan("0.7") {
							exportFlags = append(exportFlags, []string{"-run-image", exportRegFixtures.ReadOnlyRunImage}...)
						}

						exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
						exportedImageName = exportTest.RegRepoName("some-exported-image-" + h.RandString(10))
						exportArgs = append(exportArgs, exportedImageName)

						output := h.DockerRun(t,
							exportImage,
							h.WithFlags(
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_REGISTRY_AUTH="+exportRegAuthConfig,
								"--env", "SOURCE_DATE_EPOCH="+fmt.Sprintf("%d", expectedTime.Unix()),
								"--network", exportRegNetwork,
							),
							h.WithArgs(exportArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+exportedImageName)

						h.Run(t, exec.Command("docker", "pull", exportedImageName))
						assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, expectedTime)
					})
				})
				when("cache", func() {
					when("cache image case", func() {
						it("is created", func() {
							cacheImageName := exportTest.RegRepoName("some-cache-image-" + h.RandString(10))
							exportFlags := []string{"-cache-image", cacheImageName}
							if api.MustParse(platformAPI).LessThan("0.7") {
								exportFlags = append(exportFlags, "-run-image", exportRegFixtures.ReadOnlyRunImage)
							}

							exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
							exportedImageName = exportTest.RegRepoName("some-exported-image-" + h.RandString(10))
							exportArgs = append(exportArgs, exportedImageName)

							output := h.DockerRun(t,
								exportImage,
								h.WithFlags(
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--env", "CNB_REGISTRY_AUTH="+exportRegAuthConfig,
									"--network", exportRegNetwork,
								),
								h.WithArgs(exportArgs...),
							)
							h.AssertStringContains(t, output, "Saving "+exportedImageName)

							h.Run(t, exec.Command("docker", "pull", exportedImageName))
							assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, imgutil.NormalizedDateTime)
						})

						it("is created with empty layer", func() {
							cacheImageName := exportTest.RegRepoName("some-empty-cache-image-" + h.RandString(10))
							exportFlags := []string{"-cache-image", cacheImageName, "-layers", "/other_layers"}
							if api.MustParse(platformAPI).LessThan("0.7") {
								exportFlags = append(exportFlags, "-run-image", exportRegFixtures.ReadOnlyRunImage)
							}

							exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
							exportedImageName = exportTest.RegRepoName("some-exported-image-" + h.RandString(10))
							exportArgs = append(exportArgs, exportedImageName)

							output := h.DockerRun(t,
								exportImage,
								h.WithFlags(
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--env", "CNB_REGISTRY_AUTH="+exportRegAuthConfig,
									"--network", exportRegNetwork,
								),
								h.WithArgs(exportArgs...),
							)
							h.AssertStringContains(t, output, "Saving "+exportedImageName)

							testEmptyLayerSHA := calculateEmptyLayerSha(t)

							// Retrieve the cache image from the ephemeral registry
							h.Run(t, exec.Command("docker", "pull", cacheImageName))
							subject, err := cache.NewImageCacheFromName(cacheImageName, authn.DefaultKeychain)
							h.AssertNil(t, err)

							//Assert the cache image was created with an empty layer
							layer, err := subject.RetrieveLayer(testEmptyLayerSHA)
							h.AssertNil(t, err)
							defer layer.Close()
						})
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
				})

				it.After(func() {
					if h.DockerContainerExists(t, containerName) {
						h.Run(t, exec.Command("docker", "rm", containerName))
					}
					// removes all images created
					os.RemoveAll(tmpDir)
				})

				when("default layout directory", func() {
					when("first build", func() {
						when("app", func() {
							it.Before(func() {
								exportedImageName = "my-default-layout-app"
								layoutDir = filepath.Join(path.RootDir, "layout-repo")
							})

							it("is created", func() {
								var exportFlags []string
								h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "Platform API < 0.12 does not accept a -layout flag")
								exportFlags = append(exportFlags, []string{"-layout", "-layout-dir", layoutDir, "-analyzed", "/layers/layout-analyzed.toml"}...)
								exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
								exportArgs = append(exportArgs, exportedImageName)

								output := h.DockerRunAndCopy(t, containerName, tmpDir, layoutDir, exportImage,
									h.WithFlags(
										"--env", "CNB_EXPERIMENTAL_MODE=warn",
										"--env", "CNB_PLATFORM_API="+platformAPI,
									),
									h.WithArgs(exportArgs...))

								h.AssertStringContains(t, output, "Saving /layout-repo/index.docker.io/library/my-default-layout-app/latest")

								// assert the image was saved on disk in OCI layout format
								index := h.ReadIndexManifest(t, filepath.Join(tmpDir, layoutDir, "index.docker.io", "library", exportedImageName, "latest"))
								h.AssertEq(t, len(index.Manifests), 1)
							})
						})
					})
				})

				when("custom layout directory", func() {
					when("first build", func() {
						when("app", func() {
							it.Before(func() {
								exportedImageName = "my-custom-layout-app"
								layoutDir = filepath.Join(path.RootDir, "my-layout-dir")
							})

							it("is created", func() {
								var exportFlags []string
								h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "Platform API < 0.12 does not accept a -layout flag")
								exportFlags = append(exportFlags, []string{"-layout", "-layout-dir", layoutDir, "-analyzed", "/layers/layout-analyzed.toml"}...)
								exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
								exportArgs = append(exportArgs, exportedImageName)

								output := h.DockerRunAndCopy(t, containerName, tmpDir, layoutDir, exportImage,
									h.WithFlags(
										"--env", "CNB_EXPERIMENTAL_MODE=warn",
										"--env", "CNB_PLATFORM_API="+platformAPI,
									),
									h.WithArgs(exportArgs...))

								h.AssertStringContains(t, output, "Saving /my-layout-dir/index.docker.io/library/my-custom-layout-app/latest")

								// assert the image was saved on disk in OCI layout format
								index := h.ReadIndexManifest(t, filepath.Join(tmpDir, layoutDir, "index.docker.io", "library", exportedImageName, "latest"))
								h.AssertEq(t, len(index.Manifests), 1)
							})
						})
					})
				})
			})

			when("experimental mode is not enabled", func() {
				it.Before(func() {
					layoutDir = filepath.Join(path.RootDir, "layout-dir")
				})

				it("errors", func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "Platform API < 0.12 does not accept a -layout flag")

					cmd := exec.Command(
						"docker", "run", "--rm",
						"--env", "CNB_PLATFORM_API="+platformAPI,
						exportImage,
						ctrPath(exporterPath),
						"-layout",
						"-layout-dir", layoutDir,
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

func assertImageOSAndArch(t *testing.T, imageName string, phaseTest *PhaseTest) {
	inspect, _, err := h.DockerCli(t).ImageInspectWithRaw(context.TODO(), imageName)
	h.AssertNil(t, err)
	h.AssertEq(t, inspect.Os, phaseTest.targetDaemon.os)
	h.AssertEq(t, inspect.Architecture, phaseTest.targetDaemon.arch)
}

func assertImageOSAndArchAndCreatedAt(t *testing.T, imageName string, phaseTest *PhaseTest, expectedCreatedAt time.Time) {
	inspect, _, err := h.DockerCli(t).ImageInspectWithRaw(context.TODO(), imageName)
	h.AssertNil(t, err)
	h.AssertEq(t, inspect.Os, phaseTest.targetDaemon.os)
	h.AssertEq(t, inspect.Architecture, phaseTest.targetDaemon.arch)
	h.AssertEq(t, inspect.Created, expectedCreatedAt.Format(time.RFC3339))
}

func updateAnalyzedTOMLFixturesWithRegRepoName(t *testing.T, phaseTest *PhaseTest) {
	placeHolderPath := filepath.Join("testdata", "exporter", "container", "layers", "analyzed.toml.placeholder")
	analyzedMD := assertAnalyzedMetadata(t, placeHolderPath)
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: phaseTest.targetRegistry.fixtures.ReadOnlyRunImage}
	encoding.WriteTOML(strings.TrimSuffix(placeHolderPath, ".placeholder"), analyzedMD)

	placeHolderPath = filepath.Join("testdata", "exporter", "container", "layers", "some-analyzed.toml.placeholder")
	analyzedMD = assertAnalyzedMetadata(t, placeHolderPath)
	analyzedMD.PreviousImage = &platform.ImageIdentifier{Reference: phaseTest.targetRegistry.fixtures.SomeAppImage}
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: phaseTest.targetRegistry.fixtures.ReadOnlyRunImage}
	encoding.WriteTOML(strings.TrimSuffix(placeHolderPath, ".placeholder"), analyzedMD)

	placeHolderPath = filepath.Join("testdata", "exporter", "container", "other_layers", "analyzed.toml.placeholder")
	analyzedMD = assertAnalyzedMetadata(t, placeHolderPath)
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: phaseTest.targetRegistry.fixtures.ReadOnlyRunImage}
	encoding.WriteTOML(strings.TrimSuffix(placeHolderPath, ".placeholder"), analyzedMD)

	placeHolderPath = filepath.Join("testdata", "exporter", "container", "layers", "layout-analyzed.toml.placeholder")
	analyzedMD = assertAnalyzedMetadata(t, placeHolderPath)
	// Values from image acceptance/testdata/exporter/container/layout-repo in OCI layout format
	analyzedMD.RunImage = &platform.ImageIdentifier{
		Reference: "/layout-repo/index.docker.io/library/busybox/latest@sha256:445c45cc89fdeb64b915b77f042e74ab580559b8d0d5ef6950be1c0265834c33",
	}
	encoding.WriteTOML(strings.TrimSuffix(placeHolderPath, ".placeholder"), analyzedMD)
}

func calculateEmptyLayerSha(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "")
	h.AssertNil(t, err)
	testLayerEmptyPath := filepath.Join(tmpDir, "empty.tar")
	h.AssertNil(t, os.WriteFile(testLayerEmptyPath, []byte{}, 0600))
	return "sha256:" + h.ComputeSHA256ForFile(t, testLayerEmptyPath)
}
