//go:build acceptance

package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/buildpacks/imgutil"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/internal/fsutil"
	"github.com/buildpacks/lifecycle/internal/path"
	"github.com/buildpacks/lifecycle/platform/files"
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
	testImageDockerContext := filepath.Join("testdata", "exporter")
	exportTest = NewPhaseTest(t, "exporter", testImageDockerContext)

	exportTest.Start(t, updateTOMLFixturesWithTestRegistry)
	defer exportTest.Stop(t)

	exportImage = exportTest.testImageRef
	exporterPath = exportTest.containerBinaryPath
	exportRegAuthConfig = exportTest.targetRegistry.authConfig
	exportRegNetwork = exportTest.targetRegistry.network
	exportDaemonFixtures = exportTest.targetDaemon.fixtures
	exportRegFixtures = exportTest.targetRegistry.fixtures

	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "acceptance-exporter/"+platformAPI.String(), testExporterFunc(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testExporterFunc(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {

		when("daemon case", func() {
			var exportedImageName string

			it.After(func() {
				_, _, _ = h.RunE(exec.Command("docker", "rmi", exportedImageName)) // #nosec G204
			})

			it("app is created", func() {
				exportFlags := []string{"-daemon", "-log-level", "debug"}
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

				if api.MustParse(platformAPI).AtLeast("0.12") {
					expectedHistory := []string{
						"Buildpacks Launcher Config",
						"Buildpacks Application Launcher",
						"Application Layer",
						"Software Bill-of-Materials",
						"Layer: 'corrupted-layer', Created by buildpack: corrupted_buildpack@corrupted_v1",
						"Layer: 'launch-layer', Created by buildpack: cacher_buildpack@cacher_v1",
						"", // run image layer
					}
					assertDaemonImageHasHistory(t, exportedImageName, expectedHistory)
				} else {
					assertDaemonImageDoesNotHaveHistory(t, exportedImageName)
				}

				assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, imgutil.NormalizedDateTime)
			})

			when("using extensions", func() {
				it.Before(func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "")
				})

				it("app is created from the extended run image", func() {
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
					inspect, err := h.ImageInspectWithRetry(t, h.DockerCli(t), exportTest.targetRegistry.fixtures.ReadOnlyRunImage)
					h.AssertNil(t, err)
					layers := inspect.RootFS.Layers
					runImageFixtureTopLayerSHA := layers[len(layers)-1]
					runImageFixtureSHA := inspect.ID

					experimentalMode := "warn"
					if api.MustParse(platformAPI).AtLeast("0.13") {
						experimentalMode = "error"
					}

					output := h.DockerRun(t,
						exportImage,
						h.WithFlags(append(
							dockerSocketMount,
							"--env", "CNB_EXPERIMENTAL_MODE="+experimentalMode,
							"--env", "CNB_PLATFORM_API="+platformAPI,
						)...),
						h.WithArgs(exportArgs...),
					)
					h.AssertStringContains(t, output, "Saving "+exportedImageName)

					assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, imgutil.NormalizedDateTime)
					expectedHistory := []string{
						"Buildpacks Launcher Config",
						"Buildpacks Application Launcher",
						"Application Layer",
						"Software Bill-of-Materials",
						"Layer: 'corrupted-layer', Created by buildpack: corrupted_buildpack@corrupted_v1",
						"Layer: 'launch-layer', Created by buildpack: cacher_buildpack@cacher_v1",
						"Layer: 'RUN mkdir /some-other-dir && echo some-data > /some-other-dir/some-file && echo some-data > /some-other-file', Created by extension: second-extension",
						"Layer: 'RUN mkdir /some-dir && echo some-data > /some-dir/some-file && echo some-data > /some-file', Created by extension: first-extension",
						"", // run image layer
					}
					assertDaemonImageHasHistory(t, exportedImageName, expectedHistory)
					t.Log("bases the exported image on the extended run image")
					inspect, err = h.ImageInspectWithRetry(t, h.DockerCli(t), exportedImageName)
					h.AssertNil(t, err)
					h.AssertEq(t, inspect.Config.Labels["io.buildpacks.rebasable"], "false") // from testdata/exporter/container/layers/some-extended-dir/run/sha256_<sha>/blobs/sha256/<config>
					t.Log("Adds extension layers")
					type testCase struct {
						expectedDiffID string
						layerIndex     int
					}
					testCases := []testCase{
						{
							expectedDiffID: "sha256:fb54d2566824d6630d94db0b008d9a544a94d3547a424f52e2fd282b648c0601", // from testdata/exporter/container/layers/some-extended-dir/run/sha256_<c72eda1c>/blobs/sha256/65c2873d397056a5cb4169790654d787579b005f18b903082b177d4d9b4aecf5 after un-compressing and zeroing timestamps
							layerIndex:     1,
						},
						{
							expectedDiffID: "sha256:1018c7d3584c4f7fa3ef4486d1a6a11b93956b9d8bfe0898a3e0fbd248c984d8", // from testdata/exporter/container/layers/some-extended-dir/run/sha256_<c72eda1c>/blobs/sha256/0fb9b88c9cbe9f11b4c8da645f390df59f5949632985a0bfc2a842ef17b2ad18 after un-compressing and zeroing timestamps
							layerIndex:     2,
						},
					}
					for _, tc := range testCases {
						h.AssertEq(t, inspect.RootFS.Layers[tc.layerIndex], tc.expectedDiffID)
					}
					t.Log("sets the layers metadata label according to the new spec")
					var lmd files.LayersMetadata
					lmdJSON := inspect.Config.Labels["io.buildpacks.lifecycle.metadata"]
					h.AssertNil(t, json.Unmarshal([]byte(lmdJSON), &lmd))
					h.AssertEq(t, lmd.RunImage.Image, exportTest.targetRegistry.fixtures.ReadOnlyRunImage) // from analyzed.toml
					h.AssertEq(t, lmd.RunImage.Mirrors, []string{"mirror1", "mirror2"})                    // from run.toml
					h.AssertEq(t, lmd.RunImage.TopLayer, runImageFixtureTopLayerSHA)
					h.AssertEq(t, lmd.RunImage.Reference, strings.TrimPrefix(runImageFixtureSHA, "sha256:"))
				})
			})

			when("SOURCE_DATE_EPOCH is set", func() {
				it("app is created with config CreatedAt set to SOURCE_DATE_EPOCH", func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.9"), "SOURCE_DATE_EPOCH support added in 0.9")
					expectedTime := time.Date(2022, 1, 5, 5, 5, 5, 0, time.UTC)
					exportFlags := []string{"-daemon"}
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
			var exportedImageName string

			it.After(func() {
				_, _, _ = h.RunE(exec.Command("docker", "rmi", exportedImageName)) // #nosec G204
			})

			it("app is created", func() {
				var exportFlags []string
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

			when("registry is insecure", func() {
				it.Before(func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "")
				})

				it("uses http protocol", func() {
					var exportFlags []string
					exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
					exportedImageName = exportTest.RegRepoName("some-insecure-exported-image-" + h.RandString(10))
					exportArgs = append(exportArgs, exportedImageName)
					insecureRegistry := "host.docker.internal/bar"
					insecureAnalyzed := "/layers/analyzed_insecure.toml"

					_, _, err := h.DockerRunWithError(t,
						exportImage,
						h.WithFlags(
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--env", "CNB_INSECURE_REGISTRIES="+insecureRegistry,
							"--env", "CNB_ANALYZED_PATH="+insecureAnalyzed,
							"--network", exportRegNetwork,
						),
						h.WithArgs(exportArgs...),
					)
					h.AssertStringContains(t, err.Error(), "http://host.docker.internal")
				})
			})

			when("SOURCE_DATE_EPOCH is set", func() {
				it("app is created with config CreatedAt set to SOURCE_DATE_EPOCH", func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.9"), "SOURCE_DATE_EPOCH support added in 0.9")
					expectedTime := time.Date(2022, 1, 5, 5, 5, 5, 0, time.UTC)

					var exportFlags []string
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

			// FIXME: move this out of the registry block
			when("cache", func() {
				when("image case", func() {
					it("cache is created", func() {
						cacheImageName := exportTest.RegRepoName("some-cache-image-" + h.RandString(10))
						exportFlags := []string{"-cache-image", cacheImageName}
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
						// To detect whether the export of cacheImage and exportedImage is successful
						h.Run(t, exec.Command("docker", "pull", exportedImageName))
						assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, imgutil.NormalizedDateTime)
						h.Run(t, exec.Command("docker", "pull", cacheImageName))
					})

					when("parallel export is enabled", func() {
						it("cache is created", func() {
							cacheImageName := exportTest.RegRepoName("some-cache-image-" + h.RandString(10))
							exportFlags := []string{"-cache-image", cacheImageName, "-parallel"}
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
							h.Run(t, exec.Command("docker", "pull", cacheImageName))
						})
					})

					when("cache is provided but no data was cached", func() {
						it("cache is created with an empty layer", func() {
							cacheImageName := exportTest.RegRepoName("some-empty-cache-image-" + h.RandString(10))
							exportFlags := []string{"-cache-image", cacheImageName, "-layers", "/other_layers"}
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
							logger := cmd.DefaultLogger

							subject, err := cache.NewImageCacheFromName(cacheImageName, authn.DefaultKeychain, logger, cache.NewImageDeleter(cache.NewImageComparer(), logger, api.MustParse(platformAPI).LessThan("0.13")))
							h.AssertNil(t, err)

							//Assert the cache image was created with an empty layer
							layer, err := subject.RetrieveLayer(testEmptyLayerSHA)
							h.AssertNil(t, err)
							defer layer.Close()
						})
					})
				})

				when("directory case", func() {
					when("original cache was corrupted", func() {
						var cacheDir string

						it.Before(func() {
							var err error
							cacheDir, err = os.MkdirTemp("", "cache")
							h.AssertNil(t, err)
							h.AssertNil(t, os.Chmod(cacheDir, 0777)) // Override umask

							cacheFixtureDir := filepath.Join("testdata", "exporter", "cache-dir")
							h.AssertNil(t, fsutil.Copy(cacheFixtureDir, cacheDir))
							// We have to pre-create the tar files so that their digests do not change due to timestamps
							// But, ':' in the filepath on Windows is not allowed
							h.AssertNil(t, os.Rename(
								filepath.Join(cacheDir, "committed", "sha256_258dfa0cc987efebc17559694866ebc91139e7c0e574f60d1d4092f53d7dff59.tar"),
								filepath.Join(cacheDir, "committed", "sha256:258dfa0cc987efebc17559694866ebc91139e7c0e574f60d1d4092f53d7dff59.tar"),
							))
						})

						it.After(func() {
							_ = os.RemoveAll(cacheDir)
						})

						it("overwrites the original layer", func() {
							exportFlags := []string{
								"-cache-dir", "/cache",
								"-log-level", "debug",
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
									"--volume", fmt.Sprintf("%s:/cache", cacheDir),
								),
								h.WithArgs(exportArgs...),
							)
							h.AssertStringContains(t, output, "Skipping reuse for layer corrupted_buildpack:corrupted-layer: expected layer contents to have SHA 'sha256:258dfa0cc987efebc17559694866ebc91139e7c0e574f60d1d4092f53d7dff59'; found 'sha256:9e0b77ed599eafdab8611f7eeefef084077f91f02f1da0a3870c7ff20a08bee8'")
							h.AssertStringContains(t, output, "Saving "+exportedImageName)
							err := h.DockerPullWithRetry(t, exportedImageName)
							h.AssertNil(t, err)
							defer h.DockerImageRemoveSafe(t, exportedImageName)
							// Verify the app has the correct sha for the layer
							inspect, err := h.ImageInspectWithRetry(t, h.DockerCli(t), exportedImageName)
							h.AssertNil(t, err)
							var lmd files.LayersMetadata
							lmdJSON := inspect.Config.Labels["io.buildpacks.lifecycle.metadata"]
							h.AssertNil(t, json.Unmarshal([]byte(lmdJSON), &lmd))
							h.AssertEq(t, lmd.Buildpacks[2].Layers["corrupted-layer"].SHA, "sha256:258dfa0cc987efebc17559694866ebc91139e7c0e574f60d1d4092f53d7dff59")
							// Verify the cache has correct contents now
							foundDiffID, err := func() (string, error) {
								layerPath := filepath.Join(cacheDir, "committed", "sha256:258dfa0cc987efebc17559694866ebc91139e7c0e574f60d1d4092f53d7dff59.tar")
								layerRC, err := os.Open(layerPath)
								if err != nil {
									return "", err
								}
								defer func() {
									_ = layerRC.Close()
								}()
								hasher := sha256.New()
								if _, err = io.Copy(hasher, layerRC); err != nil {
									return "", errors.Wrap(err, "hashing layer")
								}
								foundDiffID := "sha256:" + hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))
								return foundDiffID, nil
							}()
							h.AssertNil(t, err)
							h.AssertEq(t, foundDiffID, "sha256:258dfa0cc987efebc17559694866ebc91139e7c0e574f60d1d4092f53d7dff59")
						})
					})
				})
			})

			when("using extensions", func() {
				it.Before(func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "")
				})

				it("app is created from the extended run image", func() {
					exportFlags := []string{
						"-analyzed", "/layers/run-image-extended-analyzed.toml",
						"-extended", "/layers/some-extended-dir",
						"-log-level", "debug",
						"-run", "/cnb/run.toml",
					}
					exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
					exportedImageName = exportTest.RegRepoName("some-exported-image-" + h.RandString(10))
					exportArgs = append(exportArgs, exportedImageName)

					// get run image SHA & top layer
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

					experimentalMode := "warn"
					if api.MustParse(platformAPI).AtLeast("0.13") {
						experimentalMode = "error"
					}

					output := h.DockerRun(t,
						exportImage,
						h.WithFlags(
							"--env", "CNB_EXPERIMENTAL_MODE="+experimentalMode,
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
					h.AssertEq(t, configFile.Config.Labels["io.buildpacks.rebasable"], "false") // from testdata/exporter/container/layers/some-extended-dir/run/sha256_<sha>/blobs/sha256/<config>
					t.Log("Adds extension layers")
					layers, err = remoteImage.Layers()
					h.AssertNil(t, err)
					type testCase struct {
						expectedDigest string
						layerIndex     int
					}
					testCases := []testCase{
						{
							expectedDigest: "sha256:08e7ad5ce17cf5e5f70affe68b341a93de86ee2ba074932c3a05b8770f66d772", // from testdata/exporter/container/layers/some-extended-dir/run/sha256_<c72eda1c>/blobs/sha256/65c2873d397056a5cb4169790654d787579b005f18b903082b177d4d9b4aecf5 after un-compressing, zeroing timestamps, and re-compressing
							layerIndex:     1,
						},
						{
							expectedDigest: "sha256:0e74ef444ea437147e3fa0ce2aad371df5380c26b96875ae07b9b67f44cdb2ee", // from testdata/exporter/container/layers/some-extended-dir/run/sha256_<c72eda1c>/blobs/sha256/0fb9b88c9cbe9f11b4c8da645f390df59f5949632985a0bfc2a842ef17b2ad18 after un-compressing, zeroing timestamps, and re-compressing
							layerIndex:     2,
						},
					}
					for _, tc := range testCases {
						layer := layers[tc.layerIndex]
						digest, err := layer.Digest()
						h.AssertNil(t, err)
						h.AssertEq(t, digest.String(), tc.expectedDigest)
					}
					t.Log("sets the layers metadata label according to the new spec")
					var lmd files.LayersMetadata
					lmdJSON := configFile.Config.Labels["io.buildpacks.lifecycle.metadata"]
					h.AssertNil(t, json.Unmarshal([]byte(lmdJSON), &lmd))
					h.AssertEq(t, lmd.RunImage.Image, exportTest.targetRegistry.fixtures.ReadOnlyRunImage) // from analyzed.toml
					h.AssertEq(t, lmd.RunImage.Mirrors, []string{"mirror1", "mirror2"})                    // from run.toml
					h.AssertEq(t, lmd.RunImage.TopLayer, runImageFixtureTopLayerSHA.String())
					h.AssertEq(t, lmd.RunImage.Reference, fmt.Sprintf("%s@%s", exportTest.targetRegistry.fixtures.ReadOnlyRunImage, runImageFixtureSHA.String()))
				})
			})
		})

		when("layout case", func() {
			var (
				containerName     string
				err               error
				layoutDir         string
				tmpDir            string
				exportedImageName string
			)

			when("experimental mode is enabled", func() {
				it.Before(func() {
					// create the directory to save all OCI images on disk
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

				when("using a custom layout directory", func() {
					it.Before(func() {
						exportedImageName = "my-custom-layout-app"
						layoutDir = filepath.Join(path.RootDir, "my-layout-dir")
					})

					it("app is created", func() {
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

func assertDaemonImageDoesNotHaveHistory(t *testing.T, repoName string) {
	history, err := h.DockerCli(t).ImageHistory(context.TODO(), repoName)
	h.AssertNil(t, err)
	for _, hs := range history {
		h.AssertEq(t, hs.Created, imgutil.NormalizedDateTime.Unix())
		h.AssertEq(t, hs.CreatedBy, "")
	}
}

func assertDaemonImageHasHistory(t *testing.T, repoName string, expectedHistory []string) {
	history, err := h.DockerCli(t).ImageHistory(context.TODO(), repoName)
	h.AssertNil(t, err)
	h.AssertEq(t, len(history), len(expectedHistory))
	for idx, hs := range history {
		h.AssertEq(t, hs.Created, imgutil.NormalizedDateTime.Unix())
		h.AssertEq(t, hs.CreatedBy, expectedHistory[idx])
	}
}

func calculateEmptyLayerSha(t *testing.T) string {
	tmpDir, err := os.MkdirTemp("", "")
	h.AssertNil(t, err)
	testLayerEmptyPath := filepath.Join(tmpDir, "empty.tar")
	h.AssertNil(t, os.WriteFile(testLayerEmptyPath, []byte{}, 0600))
	return "sha256:" + h.ComputeSHA256ForFile(t, testLayerEmptyPath)
}
