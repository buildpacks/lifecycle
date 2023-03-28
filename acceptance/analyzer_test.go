package acceptance

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/internal/path"
	"github.com/buildpacks/lifecycle/platform"
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
	rand.Seed(time.Now().UTC().UnixNano())

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

		when("called with group", func() {
			it("errors", func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.7"), "Platform API < 0.7 accepts a -group flag")
				cmd := exec.Command(
					"docker", "run", "--rm",
					"--env", "CNB_PLATFORM_API="+platformAPI,
					analyzeImage,
					ctrPath(analyzerPath),
					"-group", "group.toml",
					"some-image",
				) // #nosec G204
				output, err := cmd.CombinedOutput()

				h.AssertNotNil(t, err)
				expected := "flag provided but not defined: -group"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("called with skip layers", func() {
			it("errors", func() {
				h.SkipIf(t,
					api.MustParse(platformAPI).LessThan("0.7") || api.MustParse(platformAPI).AtLeast("0.9"),
					"Platform API < 0.7 or Platform API > 0.9 accepts a -skip-layers flag")
				cmd := exec.Command(
					"docker", "run", "--rm",
					"--env", "CNB_PLATFORM_API="+platformAPI,
					analyzeImage,
					ctrPath(analyzerPath),
					"-skip-layers",
					"some-image",
				) // #nosec G204
				output, err := cmd.CombinedOutput()

				h.AssertNotNil(t, err)
				expected := "flag provided but not defined: -skip-layers"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("called with cache dir", func() {
			it("errors", func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.7"), "Platform API < 0.7 accepts a -cache-dir flag")
				cmd := exec.Command(
					"docker", "run", "--rm",
					"--env", "CNB_PLATFORM_API="+platformAPI,
					analyzeImage,
					ctrPath(analyzerPath),
					"-cache-dir", "/cache",
					"some-image",
				) // #nosec G204
				output, err := cmd.CombinedOutput()

				h.AssertNotNil(t, err)
				expected := "flag provided but not defined: -cache-dir"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("the provided layers directory isn't writeable", func() {
			it("recursively chowns the directory", func() {
				h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

				var analyzeFlags []string
				if api.MustParse(platformAPI).AtLeast("0.7") {
					analyzeFlags = append(analyzeFlags, []string{"-run-image", analyzeRegFixtures.ReadOnlyRunImage}...)
				}

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

		when("called with group (on older platforms)", func() {
			it("uses the provided group.toml path", func() {
				h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not accept a -group flag")

				h.DockerSeedRunAndCopy(t,
					containerName,
					cacheFixtureDir, ctrPath("/cache"),
					copyDir, ctrPath("/layers"),
					analyzeImage,
					h.WithFlags(
						"--env", "CNB_PLATFORM_API="+platformAPI,
					),
					h.WithArgs(
						ctrPath(analyzerPath),
						"-cache-dir", ctrPath("/cache"),
						"-group", ctrPath("/layers/other-group.toml"),
						"some-image",
					),
				)

				h.AssertPathExists(t, filepath.Join(copyDir, "layers", "some-other-buildpack-id"))
				h.AssertPathDoesNotExist(t, filepath.Join(copyDir, "layers", "some-buildpack-id"))
			})

			when("group contains unsupported buildpacks", func() {
				it("errors", func() {
					h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not accept a -group flag")

					cmd := exec.Command(
						"docker", "run", "--rm",
						"--env", "CNB_PLATFORM_API="+platformAPI,
						analyzeImage,
						ctrPath(analyzerPath),
						"-group", ctrPath("/layers/unsupported-group.toml"),
						"some-image",
					) // #nosec G204
					output, err := cmd.CombinedOutput()

					h.AssertNotNil(t, err)
					failErr, ok := err.(*exec.ExitError)
					if !ok {
						t.Fatalf("expected an error of type exec.ExitError")
					}
					h.AssertEq(t, failErr.ExitCode(), 12) // platform code for buildpack api incompatibility
					expected := "buildpack API version '0.1' is incompatible with the lifecycle"
					h.AssertStringContains(t, string(output), expected)
				})
			})
		})

		when("called with analyzed", func() {
			it("uses the provided analyzed.toml path", func() {
				analyzeFlags := []string{"-analyzed", ctrPath("/some-dir/some-analyzed.toml")}
				if api.MustParse(platformAPI).AtLeast("0.7") {
					analyzeFlags = append(analyzeFlags, "-run-image", analyzeRegFixtures.ReadOnlyRunImage)
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
				expected := "ensure registry read access to some-run-image-from-run-toml"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		it("drops privileges", func() {
			h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

			analyzeArgs := []string{"-analyzed", "/some-dir/some-analyzed.toml"}
			if api.MustParse(platformAPI).AtLeast("0.7") {
				analyzeArgs = append(analyzeArgs, "-run-image", analyzeRegFixtures.ReadOnlyRunImage)
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
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.7"), "Platform API < 0.7 does not accept run image")

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
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.7"), "Platform API < 0.7 does not accept run image")

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
				analyzeFlags := []string{"-daemon"}
				if api.MustParse(platformAPI).AtLeast("0.7") {
					analyzeFlags = append(analyzeFlags, []string{"-run-image", "some-run-image"}...)
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
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.7"), "Platform API < 0.7 restores app metadata")

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

				it("restores app metadata to the layers directory (on older platforms)", func() {
					h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not restore app metadata")
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
							analyzeDaemonFixtures.AppImage,
						),
					)

					assertLogsAndRestoresAppMetadata(t, copyDir, output)
				})

				when("skip layers is provided", func() {
					it("writes analyzed.toml and does not write buildpack layer metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not accept a -skip-layers flag")
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
								"-skip-layers",
								analyzeDaemonFixtures.AppImage,
							),
						)

						assertAnalyzedMetadata(t, filepath.Join(copyDir, "layers", "analyzed.toml"))
						assertWritesStoreTomlOnly(t, copyDir, output)
					})
				})
			})

			when("cache is provided (on older platforms)", func() {
				when("cache image case", func() {
					when("cache image is in a daemon", func() {
						it("ignores the cache", func() {
							h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read from the cache")

							h.DockerRunAndCopy(t,
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
									"-cache-image", analyzeDaemonFixtures.CacheImage,
									"some-image",
								),
							)

							h.AssertPathDoesNotExist(t, filepath.Join(copyDir, "layers", "some-buildpack-id", "some-layer.sha"))
							h.AssertPathDoesNotExist(t, filepath.Join(copyDir, "layers", "some-buildpack-id", "some-layer.toml"))
						})
					})

					when("cache image is in a registry", func() {
						when("auth registry", func() {
							when("registry creds are provided in CNB_REGISTRY_AUTH", func() {
								it("restores cache metadata", func() {
									h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read from the cache")
									output := h.DockerRunAndCopy(t,
										containerName,
										copyDir,
										"/layers",
										analyzeImage,
										h.WithFlags(append(
											dockerSocketMount,
											"--env", "CNB_PLATFORM_API="+platformAPI,
											"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
											"--network", analyzeRegNetwork,
										)...),
										h.WithArgs(
											ctrPath(analyzerPath),
											"-daemon",
											"-cache-image", analyzeRegFixtures.SomeCacheImage,
											analyzeRegFixtures.SomeAppImage,
										),
									)

									assertLogsAndRestoresCacheMetadata(t, copyDir, output)
								})
							})

							when("registry creds are provided in the docker config.json", func() {
								it("restores cache metadata", func() {
									h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read from the cache")
									output := h.DockerRunAndCopy(t,
										containerName,
										copyDir,
										ctrPath("/layers"),
										analyzeImage,
										h.WithFlags(
											"--env", "CNB_PLATFORM_API="+platformAPI,
											"--env", "DOCKER_CONFIG=/docker-config",
											"--network", analyzeRegNetwork,
										),
										h.WithArgs(
											ctrPath(analyzerPath),
											"-cache-image",
											analyzeRegFixtures.SomeCacheImage,
											analyzeRegFixtures.SomeAppImage,
										),
									)

									assertLogsAndRestoresCacheMetadata(t, copyDir, output)
								})
							})
						})

						when("no auth registry", func() {
							it("restores cache metadata", func() {
								h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read from the cache")
								output := h.DockerRunAndCopy(t,
									containerName,
									copyDir,
									ctrPath("/layers"),
									analyzeImage,
									h.WithFlags(append(
										dockerSocketMount,
										"--env", "CNB_PLATFORM_API="+platformAPI,
										"--network", analyzeRegNetwork,
									)...),
									h.WithArgs(
										ctrPath(analyzerPath),
										"-daemon",
										"-cache-image",
										analyzeRegFixtures.ReadOnlyCacheImage,
										analyzeRegFixtures.ReadOnlyAppImage,
									),
								)

								assertLogsAndRestoresCacheMetadata(t, copyDir, output)
							})
						})
					})
				})

				when("cache directory case", func() {
					it("restores cache metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read from the cache")
						output := h.DockerSeedRunAndCopy(t,
							containerName,
							cacheFixtureDir, ctrPath("/cache"),
							copyDir, ctrPath("/layers"),
							analyzeImage,
							h.WithFlags(append(
								dockerSocketMount,
								"--env", "CNB_PLATFORM_API="+platformAPI,
							)...),
							h.WithArgs(
								ctrPath(analyzerPath),
								"-daemon",
								"-cache-dir", ctrPath("/cache"),
								"some-image",
							),
						)

						assertLogsAndRestoresCacheMetadata(t, copyDir, output)
					})

					when("the provided cache directory isn't writeable by the CNB user's group", func() {
						it("recursively chowns the directory", func() {
							h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read from the cache")
							h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

							cacheVolume := h.SeedDockerVolume(t, cacheFixtureDir)
							defer h.DockerVolumeRemove(t, cacheVolume)

							output := h.DockerRun(t,
								analyzeImage,
								h.WithFlags(append(
									dockerSocketMount,
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--volume", cacheVolume+":/cache",
								)...),
								h.WithBash(
									fmt.Sprintf("chown -R 9999:9999 /cache; chmod -R 775 /cache; %s -daemon -cache-dir /cache some-image; ls -alR /cache", analyzerPath),
								),
							)

							h.AssertMatch(t, output, "2222 3333 .+ \\.")
							h.AssertMatch(t, output, "2222 3333 .+ committed")
							h.AssertMatch(t, output, "2222 3333 .+ staging")
						})
					})

					when("the provided cache directory is writeable by the CNB user's group", func() {
						it("doesn't chown the directory", func() {
							h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")
							h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read from the cache")

							cacheVolume := h.SeedDockerVolume(t, cacheFixtureDir)
							defer h.DockerVolumeRemove(t, cacheVolume)

							output := h.DockerRun(t,
								analyzeImage,
								h.WithFlags(append(
									dockerSocketMount,
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--volume", cacheVolume+":/cache",
								)...),
								h.WithBash(
									fmt.Sprintf("chown -R 9999:3333 /cache; chmod -R 775 /cache; %s -daemon -cache-dir /cache some-image; ls -alR /cache", analyzerPath),
								),
							)

							h.AssertMatch(t, output, "9999 3333 .+ \\.")
							h.AssertMatch(t, output, "9999 3333 .+ committed")
							h.AssertMatch(t, output, "2222 3333 .+ staging")
						})
					})
				})
			})
		})

		when("registry case", func() {
			it("writes analyzed.toml", func() {
				var analyzeFlags []string
				if api.MustParse(platformAPI).AtLeast("0.7") {
					analyzeFlags = append(analyzeFlags, []string{"-run-image", analyzeRegFixtures.ReadOnlyRunImage}...)
				}

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

			when("app image exists", func() {
				when("auth registry", func() {
					when("registry creds are provided in CNB_REGISTRY_AUTH", func() {
						it("restores app metadata", func() {
							h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read app layer metadata")
							output := h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers"),
								analyzeImage,
								h.WithFlags(
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
									"--network", analyzeRegNetwork,
								),
								h.WithArgs(
									ctrPath(analyzerPath),
									analyzeRegFixtures.SomeAppImage,
								),
							)

							assertLogsAndRestoresAppMetadata(t, copyDir, output)
						})
					})

					when("registry creds are provided in the docker config.json", func() {
						it("restores app metadata", func() {
							h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read app layer metadata")
							output := h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers"),
								analyzeImage,
								h.WithFlags(
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--env", "DOCKER_CONFIG=/docker-config",
									"--network", analyzeRegNetwork,
								),
								h.WithArgs(
									ctrPath(analyzerPath),
									analyzeRegFixtures.SomeAppImage,
								),
							)

							assertLogsAndRestoresAppMetadata(t, copyDir, output)
						})
					})

					when("skip layers is provided", func() {
						it("writes analyzed.toml and does not write buildpack layer metadata", func() {
							h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not accept a -skip-layers flag")
							output := h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers"),
								analyzeImage,
								h.WithFlags(
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
									"--network", analyzeRegNetwork,
								),
								h.WithArgs(
									ctrPath(analyzerPath),
									"-skip-layers",
									analyzeRegFixtures.SomeAppImage,
								),
							)

							assertAnalyzedMetadata(t, filepath.Join(copyDir, "layers", "analyzed.toml"))
							assertWritesStoreTomlOnly(t, copyDir, output)
						})
					})
				})

				when("no auth registry", func() {
					it("restores app metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read app layer metadata")

						output := h.DockerRunAndCopy(t,
							containerName,
							copyDir,
							ctrPath("/layers"),
							analyzeImage,
							h.WithFlags(
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--network", analyzeRegNetwork,
							),
							h.WithArgs(
								ctrPath(analyzerPath),
								analyzeRegFixtures.ReadOnlyAppImage,
							),
						)

						assertLogsAndRestoresAppMetadata(t, copyDir, output)
					})

					when("skip layers is provided", func() {
						it("writes analyzed.toml and does not write buildpack layer metadata", func() {
							h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not accept a -skip-layers flag")
							output := h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers"),
								analyzeImage,
								h.WithFlags(
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--network", analyzeRegNetwork,
								),
								h.WithArgs(
									ctrPath(analyzerPath),
									"-skip-layers",
									analyzeRegFixtures.ReadOnlyAppImage,
								),
							)

							assertAnalyzedMetadata(t, filepath.Join(copyDir, "layers", "analyzed.toml"))
							assertWritesStoreTomlOnly(t, copyDir, output)
						})
					})
				})
			})

			when("called with previous image", func() {
				it.Before(func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.7"), "Platform API < 0.7 does not support -previous-image")
				})

				when("auth registry", func() {
					when("the destination image does not exist", func() {
						it("writes analyzed.toml with previous image identifier", func() {
							analyzeFlags := []string{"-previous-image", analyzeRegFixtures.ReadWriteAppImage}
							if api.MustParse(platformAPI).AtLeast("0.7") {
								analyzeFlags = append(analyzeFlags, []string{"-run-image", analyzeRegFixtures.ReadOnlyRunImage}...)
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
							analyzeFlags := []string{"-previous-image", analyzeRegFixtures.ReadWriteAppImage}
							if api.MustParse(platformAPI).AtLeast("0.7") {
								analyzeFlags = append(analyzeFlags, []string{"-run-image", analyzeRegFixtures.ReadOnlyRunImage}...)
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

			when("cache is provided (on older platforms)", func() {
				when("cache image case", func() {
					when("auth registry", func() {
						when("registry creds are provided in CNB_REGISTRY_AUTH", func() {
							it("restores cache metadata", func() {
								h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read from the cache")
								output := h.DockerRunAndCopy(t,
									containerName,
									copyDir,
									ctrPath("/layers"),
									analyzeImage,
									h.WithFlags(
										"--env", "CNB_PLATFORM_API="+platformAPI,
										"--env", "CNB_REGISTRY_AUTH="+analyzeRegAuthConfig,
										"--network", analyzeRegNetwork,
									),
									h.WithArgs(
										ctrPath(analyzerPath),
										"-cache-image", analyzeRegFixtures.SomeCacheImage,
										"some-image",
									),
								)

								assertLogsAndRestoresCacheMetadata(t, copyDir, output)
							})
						})

						when("registry creds are provided in the docker config.json", func() {
							it("restores cache metadata", func() {
								h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read from the cache")
								output := h.DockerRunAndCopy(t,
									containerName,
									copyDir,
									ctrPath("/layers"),
									analyzeImage,
									h.WithFlags(
										"--env", "CNB_PLATFORM_API="+platformAPI,
										"--env", "DOCKER_CONFIG=/docker-config",
										"--network", analyzeRegNetwork,
									),
									h.WithArgs(
										ctrPath(analyzerPath),
										"-cache-image",
										analyzeRegFixtures.SomeCacheImage,
										analyzeRegFixtures.SomeAppImage,
									),
								)

								assertLogsAndRestoresCacheMetadata(t, copyDir, output)
							})
						})
					})

					when("no auth registry", func() {
						it("restores cache metadata", func() {
							h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read from the cache")

							output := h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers"),
								analyzeImage,
								h.WithFlags(
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--network", analyzeRegNetwork,
								),
								h.WithArgs(
									ctrPath(analyzerPath),
									"-cache-image", analyzeRegFixtures.ReadOnlyCacheImage,
									analyzeRegFixtures.ReadOnlyAppImage,
								),
							)

							assertLogsAndRestoresCacheMetadata(t, copyDir, output)
						})
					})
				})

				when("cache directory case", func() {
					it("restores cache metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 does not read from the cache")
						output := h.DockerSeedRunAndCopy(t,
							containerName,
							cacheFixtureDir, ctrPath("/cache"),
							copyDir, ctrPath("/layers"),
							analyzeImage,
							h.WithFlags(
								"--env", "CNB_PLATFORM_API="+platformAPI,
							),
							h.WithArgs(
								ctrPath(analyzerPath),
								"-cache-dir", ctrPath("/cache"),
								"some-image",
							),
						)

						assertLogsAndRestoresCacheMetadata(t, copyDir, output)
					})
				})
			})

			when("called with tag", func() {
				when("read/write access to registry", func() {
					it("passes read/write validation and writes analyzed.toml", func() {
						h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.7"), "Platform API < 0.7 does not use tag flag")
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
						h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.7"), "Platform API < 0.7 does not use tag flag")
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
						expected := "validating registry write access: ensure registry read/write access to " + analyzeRegFixtures.InaccessibleImage
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

					var analyzeFlags []string
					analyzeFlags = append(analyzeFlags, []string{
						"-layout",
						"-layout-dir", layoutDir,
						"-run-image", "busybox",
					}...)
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
					reference := fmt.Sprintf("%s@%s", analyzedImagePath, "sha256:1afaac0c6907aaf5fce478e2e82c00a5ce58deca23bf34739509f29affb2c631")
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

func assertAnalyzedMetadata(t *testing.T, path string) *platform.AnalyzedMetadata {
	contents, err := os.ReadFile(path)
	h.AssertNil(t, err)
	h.AssertEq(t, len(contents) > 0, true)

	analyzedMD, err := platform.ReadAnalyzed(path, cmd.DefaultLogger)
	h.AssertNil(t, err)

	return &analyzedMD
}

func assertLogsAndRestoresAppMetadata(t *testing.T, dir, output string) {
	layerFilenames := []string{
		"launch-layer.sha",
		"launch-layer.toml",
		"store.toml",
	}
	for _, filename := range layerFilenames {
		h.AssertPathExists(t, filepath.Join(dir, "layers", "some-buildpack-id", filename))
	}
	layerNames := []string{
		"launch-layer",
	}
	for _, layerName := range layerNames {
		h.AssertStringContains(t, output, fmt.Sprintf("Restoring metadata for \"some-buildpack-id:%s\"", layerName))
	}
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

func assertLogsAndRestoresCacheMetadata(t *testing.T, dir, output string) {
	h.AssertPathExists(t, filepath.Join(dir, "layers", "some-buildpack-id", "some-layer.sha"))
	h.AssertPathExists(t, filepath.Join(dir, "layers", "some-buildpack-id", "some-layer.toml"))
	h.AssertStringContains(t, output, "Restoring metadata for \"some-buildpack-id:some-layer\" from cache")
}

func assertWritesStoreTomlOnly(t *testing.T, dir, output string) {
	h.AssertPathExists(t, filepath.Join(dir, "layers", "some-buildpack-id", "store.toml"))
	layerFilenames := []string{
		"launch-build-cache-layer.sha",
		"launch-build-cache-layer.toml",
		"launch-cache-layer.sha",
		"launch-cache-layer.toml",
		"launch-layer.sha",
		"launch-layer.toml",
	}
	for _, filename := range layerFilenames {
		h.AssertPathDoesNotExist(t, filepath.Join(dir, "layers", "some-buildpack-id", filename))
	}
	h.AssertStringContains(t, output, "Skipping buildpack layer analysis")
}

func flatPrint(arr []string) string {
	return strings.Join(arr, " ")
}
