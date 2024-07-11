package acceptance

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/internal/path"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	analyzeImage          string
	analyzeRegAuthConfig  string
	analyzeRegNetwork     string
	analyzerPath          string
	analyzeDaemonFixtures *daemonImageFixtures
	analyzeRegFixtures    *regImageFixtures
	analyzeTest           *PhaseTest
)

func TestAnalyzer(t *testing.T) {
	testImageDockerContext := filepath.Join("testdata", "analyzer")
	analyzeTest = NewPhaseTest(t, "analyzer", testImageDockerContext)
	analyzeTest.Start(t)
	defer analyzeTest.Stop(t)

	analyzeImage = analyzeTest.testImageRef
	analyzerPath = analyzeTest.containerBinaryPath
	cacheFixtureDir = filepath.Join("testdata", "cache-dir")
	analyzeRegAuthConfig = analyzeTest.targetRegistry.authConfig
	analyzeRegNetwork = analyzeTest.targetRegistry.network
	analyzeDaemonFixtures = analyzeTest.targetDaemon.fixtures
	analyzeRegFixtures = analyzeTest.targetRegistry.fixtures

	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "acceptance-analyzer/"+platformAPI.String(), testAnalyzerFunc(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testAnalyzerFunc(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
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

		when("CNB_PLATFORM_API not provided", func() {
			it("errors", func() {
				cmd := exec.Command(
					"docker", "run", "--rm",
					"--env", "CNB_PLATFORM_API= ",
					analyzeImage,
					ctrPath(analyzerPath),
					"some-image",
				) // #nosec G204
				output, err := cmd.CombinedOutput()

				h.AssertNotNil(t, err)
				expected := "please set 'CNB_PLATFORM_API'"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("called without an app image", func() {
			it("errors", func() {
				cmd := exec.Command(
					"docker", "run", "--rm",
					"--env", "CNB_PLATFORM_API="+platformAPI,
					analyzeImage,
					ctrPath(analyzerPath),
				) // #nosec G204
				output, err := cmd.CombinedOutput()

				h.AssertNotNil(t, err)
				expected := "failed to parse arguments: received 0 arguments, but expected 1"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("called with skip layers", func() {
			it("writes analyzed.toml and does not restore previous image SBOM", func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.9"), "Platform API < 0.9 does not accept a -skip-layers flag")
				output := h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					ctrPath("/layers"),
					analyzeImage,
					h.WithFlags(append(
						dockerSocketMount,
						"--env", "CNB_PLATFORM_API="+platformAPI,
					)...),
					h.WithArgs(
						ctrPath(analyzerPath),
						"-daemon",
						"-run-image", analyzeRegFixtures.ReadOnlyRunImage,
						"-skip-layers",
						analyzeDaemonFixtures.AppImage,
					),
				)
				assertAnalyzedMetadata(t, filepath.Join(copyDir, "layers", "analyzed.toml"))
				h.AssertStringDoesNotContain(t, output, "Restoring data for SBOM from previous image")
			})
		})

		when("the provided layers directory isn't writeable", func() {
			it("recursively chowns the directory", func() {
				h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

				analyzeFlags := []string{"-run-image", analyzeRegFixtures.ReadOnlyRunImage}

				output := h.DockerRun(t,
					analyzeImage,
					h.WithFlags(
						"--env", "CNB_PLATFORM_API="+platformAPI,
						"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
						"--network", analyzeRegNetwork,
					),
					h.WithBash(
						fmt.Sprintf("chown -R 9999:9999 /layers; chmod -R 775 /layers; %s %s %s; ls -al /layers",
							analyzerPath,
							flatPrint(analyzeFlags),
							analyzeRegFixtures.SomeAppImage),
					),
				)

				h.AssertMatch(t, output, "2222 3333 .+ \\.")
				h.AssertMatch(t, output, "2222 3333 .+ group.toml")
			})
		})

		when("called with analyzed", func() {
			it("uses the provided analyzed.toml path", func() {
				analyzeFlags := []string{
					"-analyzed", ctrPath("/some-dir/some-analyzed.toml"),
					"-run-image", analyzeRegFixtures.ReadOnlyRunImage,
				}

				var execArgs []string
				execArgs = append([]string{ctrPath(analyzerPath)}, analyzeFlags...)
				execArgs = append(execArgs, analyzeRegFixtures.SomeAppImage)

				h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					ctrPath("/some-dir/some-analyzed.toml"),
					analyzeImage,
					h.WithFlags(
						"--env", "CNB_PLATFORM_API="+platformAPI,
						"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
						"--network", analyzeRegNetwork,
					),
					h.WithArgs(execArgs...),
				)

				assertAnalyzedMetadata(t, filepath.Join(copyDir, "some-analyzed.toml"))
			})
		})

		when("called with run", func() {
			it("uses the provided run.toml path", func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "Platform API < 0.12 does not accept -run")
				cmd := exec.Command(
					"docker", "run", "--rm",
					"--env", "CNB_PLATFORM_API="+platformAPI,
					"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
					"--network", analyzeRegNetwork,
					analyzeImage,
					ctrPath(analyzerPath),
					"-run", "/cnb/run.toml",
					analyzeRegFixtures.SomeAppImage,
				) // #nosec G204
				output, err := cmd.CombinedOutput()

				h.AssertNotNil(t, err)
				expected := "failed to find accessible run image"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		it("drops privileges", func() {
			h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

			analyzeArgs := []string{
				"-analyzed", "/some-dir/some-analyzed.toml",
				"-run-image", analyzeRegFixtures.ReadOnlyRunImage,
			}

			output := h.DockerRun(t,
				analyzeImage,
				h.WithFlags(
					"--env", "CNB_PLATFORM_API="+platformAPI,
					"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
					"--network", analyzeRegNetwork,
				),
				h.WithBash(
					fmt.Sprintf("%s %s %s; ls -al /some-dir",
						ctrPath(analyzerPath),
						flatPrint(analyzeArgs),
						analyzeRegFixtures.SomeAppImage,
					),
				),
			)

			h.AssertMatch(t, output, "2222 3333 .+ some-analyzed.toml")
		})

		when("run image", func() {
			when("provided", func() {
				it("is recorded in analyzed.toml", func() {
					h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						ctrPath("/layers/analyzed.toml"),
						analyzeImage,
						h.WithFlags(
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
							"--network", analyzeRegNetwork,
						),
						h.WithArgs(ctrPath(analyzerPath), "-run-image", analyzeRegFixtures.ReadOnlyRunImage, analyzeRegFixtures.SomeAppImage),
					)

					analyzedMD := assertAnalyzedMetadata(t, filepath.Join(copyDir, "analyzed.toml"))
					h.AssertStringContains(t, analyzedMD.RunImage.Reference, analyzeRegFixtures.ReadOnlyRunImage+"@sha256:")
				})
			})

			when("not provided", func() {
				it("falls back to CNB_RUN_IMAGE", func() {
					h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						ctrPath("/layers/analyzed.toml"),
						analyzeImage,
						h.WithFlags(
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
							"--env", "CNB_RUN_IMAGE="+analyzeRegFixtures.ReadOnlyRunImage,
							"--network", analyzeRegNetwork,
						),
						h.WithArgs(ctrPath(analyzerPath), analyzeRegFixtures.SomeAppImage),
					)

					analyzedMD := assertAnalyzedMetadata(t, filepath.Join(copyDir, "analyzed.toml"))
					h.AssertStringContains(t, analyzedMD.RunImage.Reference, analyzeRegFixtures.ReadOnlyRunImage+"@sha256:")
				})
			})
		})

		when("daemon case", func() {
			it("writes analyzed.toml", func() {
				analyzeFlags := []string{
					"-daemon",
					"-run-image", "some-run-image",
				}

				var execArgs []string
				execArgs = append([]string{ctrPath(analyzerPath)}, analyzeFlags...)
				execArgs = append(execArgs, analyzeRegFixtures.ReadOnlyAppImage)

				h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					ctrPath("/layers/analyzed.toml"),
					analyzeImage,
					h.WithFlags(append(
						dockerSocketMount,
						"--env", "CNB_PLATFORM_API="+platformAPI,
					)...),
					h.WithArgs(execArgs...),
				)

				assertAnalyzedMetadata(t, filepath.Join(copyDir, "analyzed.toml"))
			})

			when("app image exists", func() {
				it("does not restore app metadata to the layers directory", func() {
					analyzeFlags := []string{"-daemon", "-run-image", "some-run-image"}

					var execArgs []string
					execArgs = append([]string{ctrPath(analyzerPath)}, analyzeFlags...)
					execArgs = append(execArgs, analyzeDaemonFixtures.AppImage)

					output := h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						ctrPath("/layers"),
						analyzeImage,
						h.WithFlags(append(
							dockerSocketMount,
							"--env", "CNB_PLATFORM_API="+platformAPI,
						)...),
						h.WithArgs(execArgs...),
					)

					assertNoRestoreOfAppMetadata(t, copyDir, output)
				})
			})
		})

		when("registry case", func() {
			it("writes analyzed.toml", func() {
				analyzeFlags := []string{"-run-image", analyzeRegFixtures.ReadOnlyRunImage}

				var execArgs []string
				execArgs = append([]string{ctrPath(analyzerPath)}, analyzeFlags...)
				execArgs = append(execArgs, analyzeRegFixtures.SomeAppImage)

				h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					ctrPath("/layers/analyzed.toml"),
					analyzeImage,
					h.WithFlags(
						"--env", "CNB_PLATFORM_API="+platformAPI,
						"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
						"--network", analyzeRegNetwork,
					),
					h.WithArgs(execArgs...),
				)

				assertAnalyzedMetadata(t, filepath.Join(copyDir, "analyzed.toml"))
			})

			when("called with previous image", func() {
				when("auth registry", func() {
					when("the destination image does not exist", func() {
						it("writes analyzed.toml with previous image identifier", func() {
							analyzeFlags := []string{
								"-previous-image", analyzeRegFixtures.ReadWriteAppImage,
								"-run-image", analyzeRegFixtures.ReadOnlyRunImage,
							}

							var execArgs []string
							execArgs = append([]string{ctrPath(analyzerPath)}, analyzeFlags...)
							execArgs = append(execArgs, analyzeRegFixtures.ReadWriteOtherAppImage)

							h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers/analyzed.toml"),
								analyzeImage,
								h.WithFlags(
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
									"--network", analyzeRegNetwork,
								),
								h.WithArgs(execArgs...),
							)
							analyzedMD := assertAnalyzedMetadata(t, filepath.Join(copyDir, "analyzed.toml"))
							h.AssertStringContains(t, analyzedMD.PreviousImageRef(), analyzeRegFixtures.ReadWriteAppImage)
						})
					})

					when("the destination image exists", func() {
						it("writes analyzed.toml with previous image identifier", func() {
							analyzeFlags := []string{
								"-previous-image", analyzeRegFixtures.ReadWriteAppImage,
								"-run-image", analyzeRegFixtures.ReadOnlyRunImage,
							}

							var execArgs []string
							execArgs = append([]string{ctrPath(analyzerPath)}, analyzeFlags...)
							execArgs = append(execArgs, analyzeRegFixtures.ReadWriteOtherAppImage)

							h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers/analyzed.toml"),
								analyzeImage,
								h.WithFlags(
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
									"--network", analyzeRegNetwork,
								),
								h.WithArgs(execArgs...),
							)

							analyzedMD := assertAnalyzedMetadata(t, filepath.Join(copyDir, "analyzed.toml"))
							h.AssertStringContains(t, analyzedMD.PreviousImageRef(), analyzeRegFixtures.ReadWriteAppImage)
						})
					})
				})
			})

			when("called with tag", func() {
				when("read/write access to registry", func() {
					it("passes read/write validation and writes analyzed.toml", func() {
						execArgs := []string{
							ctrPath(analyzerPath),
							"-tag", analyzeRegFixtures.ReadWriteOtherAppImage,
							analyzeRegFixtures.ReadWriteAppImage,
						}
						h.DockerRunAndCopy(t,
							containerName,
							copyDir,
							ctrPath("/layers/analyzed.toml"),
							analyzeImage,
							h.WithFlags(
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
								"--env", "CNB_RUN_IMAGE="+analyzeRegFixtures.ReadOnlyRunImage,
								"--network", analyzeRegNetwork,
							),
							h.WithArgs(execArgs...),
						)
						analyzedMD := assertAnalyzedMetadata(t, filepath.Join(copyDir, "analyzed.toml"))
						h.AssertStringContains(t, analyzedMD.PreviousImageRef(), analyzeRegFixtures.ReadWriteAppImage)
					})
				})

				when("no read/write access to registry", func() {
					it("throws read/write error accessing destination tag", func() {
						cmd := exec.Command(
							"docker", "run", "--rm",
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--env", "CNB_RUN_IMAGE="+analyzeRegFixtures.ReadOnlyRunImage,
							"--name", containerName,
							"--network", analyzeRegNetwork,
							analyzeImage,
							ctrPath(analyzerPath),
							"-tag", analyzeRegFixtures.InaccessibleImage,
							analyzeRegFixtures.ReadWriteAppImage,
						) // #nosec G204
						output, err := cmd.CombinedOutput()

						h.AssertNotNil(t, err)
						expected := "ensure registry read/write access to " + analyzeRegFixtures.InaccessibleImage
						h.AssertStringContains(t, string(output), expected)
					})
				})
			})
		})

		when("layout case", func() {
			layoutDir := filepath.Join(path.RootDir, "layout-repo")
			when("experimental mode is enabled", func() {
				it("writes analyzed.toml", func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "Platform API < 0.12 does not accept a -layout flag")

					analyzeFlags := []string{
						"-layout",
						"-layout-dir", layoutDir,
						"-run-image", "busybox",
					}
					var execArgs []string
					execArgs = append([]string{ctrPath(analyzerPath)}, analyzeFlags...)
					execArgs = append(execArgs, "my-app")

					h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						ctrPath("/layers/analyzed.toml"),
						analyzeImage,
						h.WithFlags(
							"--env", "CNB_EXPERIMENTAL_MODE=warn",
							"--env", "CNB_PLATFORM_API="+platformAPI,
						),
						h.WithArgs(execArgs...),
					)

					analyzer := assertAnalyzedMetadata(t, filepath.Join(copyDir, "analyzed.toml"))
					h.AssertNotNil(t, analyzer.RunImage)
					analyzedImagePath := filepath.Join(path.RootDir, "layout-repo", "index.docker.io", "library", "busybox", "latest")
					reference := fmt.Sprintf("%s@%s", analyzedImagePath, "sha256:f75f3d1a317fc82c793d567de94fc8df2bece37acd5f2bd364a0d91a0d1f3dab")
					h.AssertEq(t, analyzer.RunImage.Reference, reference)
				})
			})

			when("experimental mode is not enabled", func() {
				it("errors", func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "Platform API < 0.12 does not accept a -layout flag")
					cmd := exec.Command(
						"docker", "run", "--rm",
						"--env", "CNB_PLATFORM_API="+platformAPI,
						"--env", "CNB_LAYOUT_DIR="+layoutDir,
						analyzeImage,
						ctrPath(analyzerPath),
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

func assertAnalyzedMetadata(t *testing.T, path string) *files.Analyzed {
	contents, err := os.ReadFile(path)
	h.AssertNil(t, err)
	h.AssertEq(t, len(contents) > 0, true)

	analyzedMD, err := files.Handler.ReadAnalyzed(path, cmd.DefaultLogger)
	h.AssertNil(t, err)

	return &analyzedMD
}

func assertNoRestoreOfAppMetadata(t *testing.T, dir, output string) {
	layerFilenames := []string{
		"launch-build-cache-layer.sha",
		"launch-build-cache-layer.toml",
		"launch-cache-layer.sha",
		"launch-cache-layer.toml",
		"launch-layer.sha",
		"launch-layer.toml",
		"store.toml",
	}
	for _, filename := range layerFilenames {
		h.AssertPathDoesNotExist(t, filepath.Join(dir, "layers", "some-buildpack-id", filename))
	}
}

func flatPrint(arr []string) string {
	return strings.Join(arr, " ")
}
