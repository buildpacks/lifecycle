//go:build acceptance
// +build acceptance

package acceptance

import (
	"context"
	"encoding/json"
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
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
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

	exportTest.Start(t, updateTOMLFixturesWithTestRegistry)
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
							exportFlags = append(exportFlags, []string{"-run-image", exportRegFixtures.ReadOnlyRunImage}...) // though the run image is registry image, it also exists in the daemon with the same tag
						}

						exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
						exportedImageName = "some-exported-image-" + h.RandString(10)
						exportArgs = append(exportArgs, exportedImageName)

						output := h.DockerRun(t,
							exportImage,
							h.WithFlags(append(
								dockerSocketMount,
								"--env", "CNB_PLATFORM_API="+platformAPI,
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

				when("using extensions", func() {
					it.Before(func() {
						h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "")
					})

					it("is created from the extended run image", func() {
						exportFlags := []string{
							"-analyzed", "/layers/run-image-extended-analyzed.toml", // though the run image is a registry image, it also exists in the daemon with the same tag
							"-daemon",
							"-extended", "/layers/some-extended-dir",
							"-log-level", "debug",
							"-run", "/cnb/run.toml", // though the run image is a registry image, it also exists in the daemon with the same tag
						}
						exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
						exportedImageName = "some-exported-image-" + h.RandString(10)
						exportArgs = append(exportArgs, exportedImageName)

						// get run image top layer
						inspect, _, err := h.DockerCli(t).ImageInspectWithRaw(context.TODO(), exportTest.targetRegistry.fixtures.ReadOnlyRunImage)
						h.AssertNil(t, err)
						layers := inspect.RootFS.Layers
						runImageFixtureTopLayerSHA := layers[len(layers)-1]
						runImageFixtureSHA := inspect.ID

						output := h.DockerRun(t,
							exportImage,
							h.WithFlags(append(
								dockerSocketMount,
								"--env", "CNB_EXPERIMENTAL_MODE=warn",
								"--env", "CNB_PLATFORM_API="+platformAPI,
							)...),
							h.WithArgs(exportArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+exportedImageName)

						assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, imgutil.NormalizedDateTime)
						t.Log("bases the exported image on the extended run image")
						inspect, _, err = h.DockerCli(t).ImageInspectWithRaw(context.TODO(), exportedImageName)
						h.AssertNil(t, err)
						h.AssertEq(t, inspect.Config.Labels["io.buildpacks.rebasable"], "false") // from testdata/exporter/container/layers/extended/sha256:<sha>/blobs/sha256/<config>
						t.Log("Adds extension layers")
						diffIDFromExt1 := "sha256:60600f423214c27fd184ebc96ae765bf2b4703c9981fb4205d28dd35e7eec4ae"
						diffIDFromExt2 := "sha256:1d811b70500e2e9a5e5b8ca7429ef02e091cdf4657b02e456ec54dd1baea0a66"
						var foundFromExt1, foundFromExt2 bool
						for _, layer := range inspect.RootFS.Layers {
							if layer == diffIDFromExt1 {
								foundFromExt1 = true
							}
							if layer == diffIDFromExt2 {
								foundFromExt2 = true
							}
						}
						h.AssertEq(t, foundFromExt1, true)
						h.AssertEq(t, foundFromExt2, true)
						t.Log("sets the layers metadata label according to the new spec")
						var lmd platform.LayersMetadata
						lmdJSON := inspect.Config.Labels["io.buildpacks.lifecycle.metadata"]
						h.AssertNil(t, json.Unmarshal([]byte(lmdJSON), &lmd))
						h.AssertEq(t, lmd.RunImage.Image, exportTest.targetRegistry.fixtures.ReadOnlyRunImage) // from analyzed.toml
						h.AssertEq(t, lmd.RunImage.Mirrors, []string{"mirror1", "mirror2"})                    // from run.toml
						h.AssertEq(t, lmd.RunImage.TopLayer, runImageFixtureTopLayerSHA)
						h.AssertEq(t, lmd.RunImage.Reference, strings.TrimPrefix(runImageFixtureSHA, "sha256:"))
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
							subject, err := cache.NewImageCacheFromName(cacheImageName, authn.DefaultKeychain, cmd.DefaultLogger)
							h.AssertNil(t, err)

							//Assert the cache image was created with an empty layer
							layer, err := subject.RetrieveLayer(testEmptyLayerSHA)
							h.AssertNil(t, err)
							defer layer.Close()
						})
					})
				})

				when("using extensions", func() {
					it.Before(func() {
						h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "")
					})

					it("is created from the extended run image", func() {
						exportFlags := []string{
							"-analyzed", "/layers/run-image-extended-analyzed.toml",
							"-extended", "/layers/some-extended-dir",
							"-log-level", "debug",
							"-run", "/cnb/run.toml",
						}
						exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
						exportedImageName = exportTest.RegRepoName("some-exported-image-" + h.RandString(10))
						exportArgs = append(exportArgs, exportedImageName)

						// get run image top layer
						ref, imageAuth, err := auth.ReferenceForRepoName(authn.DefaultKeychain, exportTest.targetRegistry.fixtures.ReadOnlyRunImage)
						h.AssertNil(t, err)
						remoteImage, err := remote.Image(ref, remote.WithAuth(imageAuth))
						h.AssertNil(t, err)
						layers, err := remoteImage.Layers()
						h.AssertNil(t, err)
						runImageFixtureTopLayerSHA, err := layers[len(layers)-1].DiffID()
						h.AssertNil(t, err)
						runImageFixtureSHA, err := remoteImage.Digest()
						h.AssertNil(t, err)

						output := h.DockerRun(t,
							exportImage,
							h.WithFlags(
								"--env", "CNB_EXPERIMENTAL_MODE=warn",
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_REGISTRY_AUTH="+exportRegAuthConfig,
								"--network", exportRegNetwork,
							),
							h.WithArgs(exportArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+exportedImageName)

						h.Run(t, exec.Command("docker", "pull", exportedImageName))
						assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, imgutil.NormalizedDateTime)
						t.Log("bases the exported image on the extended run image")
						ref, imageAuth, err = auth.ReferenceForRepoName(authn.DefaultKeychain, exportedImageName)
						h.AssertNil(t, err)
						remoteImage, err = remote.Image(ref, remote.WithAuth(imageAuth))
						h.AssertNil(t, err)
						configFile, err := remoteImage.ConfigFile()
						h.AssertNil(t, err)
						h.AssertEq(t, configFile.Config.Labels["io.buildpacks.rebasable"], "false") // from testdata/exporter/container/layers/extended/sha256:<sha>/blobs/sha256/<config>
						t.Log("Adds extension layers")
						layers, err = remoteImage.Layers()
						h.AssertNil(t, err)
						digestFromExt1 := "sha256:0c5f7a6fe14dbd19670f39e7466051cbd40b3a534c0812659740fb03e2137c1a"
						digestFromExt2 := "sha256:482346d1e0c7afa2514ec366d2e000e0667d0a6664690aab3c8ad51c81915b91"
						var foundFromExt1, foundFromExt2 bool
						for _, layer := range layers {
							digest, err := layer.Digest()
							h.AssertNil(t, err)
							if digest.String() == digestFromExt1 {
								foundFromExt1 = true
							}
							if digest.String() == digestFromExt2 {
								foundFromExt2 = true
							}
						}
						h.AssertEq(t, foundFromExt1, true)
						h.AssertEq(t, foundFromExt2, true)
						t.Log("sets the layers metadata label according to the new spec")
						var lmd platform.LayersMetadata
						lmdJSON := configFile.Config.Labels["io.buildpacks.lifecycle.metadata"]
						h.AssertNil(t, json.Unmarshal([]byte(lmdJSON), &lmd))
						h.AssertEq(t, lmd.RunImage.Image, exportTest.targetRegistry.fixtures.ReadOnlyRunImage) // from analyzed.toml
						h.AssertEq(t, lmd.RunImage.Mirrors, []string{"mirror1", "mirror2"})                    // from run.toml
						h.AssertEq(t, lmd.RunImage.TopLayer, runImageFixtureTopLayerSHA.String())
						h.AssertEq(t, lmd.RunImage.Reference, fmt.Sprintf("%s@%s", exportTest.targetRegistry.fixtures.ReadOnlyRunImage, runImageFixtureSHA.String()))
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

func calculateEmptyLayerSha(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "")
	h.AssertNil(t, err)
	testLayerEmptyPath := filepath.Join(tmpDir, "empty.tar")
	h.AssertNil(t, os.WriteFile(testLayerEmptyPath, []byte{}, 0600))
	return "sha256:" + h.ComputeSHA256ForFile(t, testLayerEmptyPath)
}
