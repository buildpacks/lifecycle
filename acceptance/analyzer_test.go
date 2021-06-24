package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	ih "github.com/buildpacks/imgutil/testhelpers"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	analyzerBinaryDir    = filepath.Join("testdata", "analyzer", "analyze-image", "container", "cnb", "lifecycle")
	analyzeDockerContext = filepath.Join("testdata", "analyzer", "analyze-image")
	analyzeImage         = "lifecycle/acceptance/analyzer"
	analyzerPath         = "/cnb/lifecycle/analyzer"
	cacheFixtureDir      = filepath.Join("testdata", "analyzer", "cache-dir")
	daemonOS             string
	noAuthRegistry       *ih.DockerRegistry
	authRegistry         *ih.DockerRegistry
	registryNetwork      string
)

func TestAnalyzer(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	info, err := h.DockerCli(t).Info(context.TODO())
	h.AssertNil(t, err)
	daemonOS = info.OSType

	// Setup registry

	dockerConfigDir, err := ioutil.TempDir("", "test.docker.config.dir")
	h.AssertNil(t, err)
	defer os.RemoveAll(dockerConfigDir)

	sharedRegHandler := registry.New(registry.Logger(log.New(ioutil.Discard, "", log.Lshortfile)))
	authRegistry = ih.NewDockerRegistry(ih.WithAuth(dockerConfigDir), ih.WithSharedHandler(sharedRegHandler))
	authRegistry.Start(t)
	defer authRegistry.Stop(t)

	noAuthRegistry = ih.NewDockerRegistry(ih.WithSharedHandler(sharedRegHandler))
	noAuthRegistry.Start(t)
	defer noAuthRegistry.Stop(t)

	// if registry is listening on localhost, use host networking to allow containers to reach it
	registryNetwork = "default"
	if authRegistry.Host == "localhost" {
		registryNetwork = "host"
	}

	os.Setenv("DOCKER_CONFIG", authRegistry.DockerDirectory)
	// Copy docker config directory to analyze-image container
	targetDockerConfig := filepath.Join("testdata", "analyzer", "analyze-image", "container", "docker-config")
	h.AssertNil(t, os.RemoveAll(filepath.Join(targetDockerConfig, "config.json")))
	h.RecursiveCopy(t, authRegistry.DockerDirectory, targetDockerConfig)

	// build run-images into test registry
	runImageContext := filepath.Join("testdata", "analyzer", "run-image")
	buildAuthRegistryImage(
		t,
		"company/stack:bionic",
		runImageContext,
		"-f", filepath.Join(runImageContext, dockerfileName),
		"--build-arg", "stackid=io.buildpacks.stacks.bionic",
	)
	buildAuthRegistryImage(
		t,
		"company/stack:centos",
		runImageContext,
		"-f", filepath.Join(runImageContext, dockerfileName),
		"--build-arg", "stackid=io.company.centos",
	)

	// build run-image into daemon
	h.DockerBuild(
		t,
		"localcompany/stack:bionic",
		runImageContext,
		h.WithArgs(
			"-f", filepath.Join(runImageContext, dockerfileName),
			"--build-arg", "stackid=io.buildpacks.stacks.bionic",
		),
	)

	defer h.DockerImageRemove(t, "localcompany/stack:bionic")

	// Setup test container

	h.MakeAndCopyLifecycle(t, daemonOS, analyzerBinaryDir)
	h.DockerBuild(t,
		analyzeImage,
		analyzeDockerContext,
		h.WithFlags(
			"-f", filepath.Join(analyzeDockerContext, dockerfileName),
			"--build-arg", "registry="+noAuthRegistry.Host+":"+noAuthRegistry.Port,
		),
	)
	defer h.DockerImageRemove(t, analyzeImage)

	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "acceptance-analyzer/"+platformAPI.String(), testAnalyzerFunc(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testAnalyzerFunc(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var copyDir, containerName, cacheVolume, basicAuth string

		it.Before(func() {
			containerName = "test-container-" + h.RandString(10)
			var err error
			copyDir, err = ioutil.TempDir("", "test-docker-copy-")
			h.AssertNil(t, err)
			basicAuth = getBasicAuth()
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
				h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0, "Platform API < 0.7 accepts a -group flag")
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
				h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0, "Platform API < 0.7 accepts a -skip-layers flag")
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
				h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0, "Platform API < 0.7 accepts a -cache-dir flag")
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

		when("cache image tag and cache directory are both blank", func() {
			it("warns", func() {
				h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not warn because it does not accept a -cache-dir flag")
				output := h.DockerRun(t,
					analyzeImage,
					h.WithFlags("--env", "CNB_PLATFORM_API="+platformAPI),
					h.WithArgs(
						ctrPath(analyzerPath),
						"some-image",
					),
				)

				expected := "Not restoring cached layer metadata, no cache flag specified."
				h.AssertStringContains(t, output, expected)
			})
		})

		when("the provided layers directory isn't writeable", func() {
			it("recursively chowns the directory", func() {
				h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")
				output := h.DockerRun(t,
					analyzeImage,
					h.WithFlags("--env", "CNB_PLATFORM_API="+platformAPI),
					h.WithBash(fmt.Sprintf("chown -R 9999:9999 /layers; chmod -R 775 /layers; %s %s; ls -al /layers", analyzerPath,
						noAuthRegistry.RepoName("some-image"))),
				)

				h.AssertMatch(t, output, "2222 3333 .+ \\.")
				h.AssertMatch(t, output, "2222 3333 .+ group.toml")
			})
		})

		when("group path is provided", func() {
			it("uses the provided group path", func() {
				h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not accept a -group flag")
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
		})

		when("analyzed path is provided", func() {
			it("writes analyzed.toml at the provided path", func() {
				execArgs := []string{
					ctrPath(analyzerPath),
					"-analyzed", ctrPath("/some-dir/some-analyzed.toml"),
					noAuthRegistry.RepoName("some-image"),
				}

				h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					ctrPath("/some-dir/some-analyzed.toml"),
					analyzeImage,
					h.WithFlags(
						"--network", registryNetwork,
						"--env", "CNB_PLATFORM_API="+platformAPI,
					),
					h.WithArgs(execArgs...),
				)

				assertAnalyzedMetadata(t, filepath.Join(copyDir, "some-analyzed.toml"))
			})
		})

		when("daemon case", func() {
			it("writes analyzed.toml", func() {
				execArgs := []string{
					ctrPath(analyzerPath),
					"-daemon",
					noAuthRegistry.RepoName("some-image"),
				}

				h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					ctrPath("/layers/analyzed.toml"),
					analyzeImage,
					h.WithFlags(append(
						dockerSocketMount,
						"--env", "CNB_PLATFORM_API="+platformAPI,
						"--env", "CNB_STACK_PATH=/cnb/local-bionic-stack.toml", // /cnb/local-bionic-stack.toml has `io.buildpacks.stacks.bionic` and points to run image `localcompany/stack:bionic` with same stack id
					)...),
					h.WithArgs(execArgs...),
				)

				assertAnalyzedMetadata(t, filepath.Join(copyDir, "analyzed.toml"))
			})

			when("app image exists", func() {
				var appImage string

				it.Before(func() {
					appImage = "some-app-image-" + h.RandString(10)
					metadata := minifyMetadata(t, filepath.Join("testdata", "analyzer", "app_image_metadata.json"), platform.LayersMetadata{})

					cmd := exec.Command(
						"docker",
						"build",
						"-t", appImage,
						"--build-arg", "fromImage="+containerBaseImage,
						"--build-arg", "metadata="+metadata,
						filepath.Join("testdata", "analyzer", "app-image"),
					)
					h.Run(t, cmd)
				})

				it.After(func() {
					h.DockerImageRemove(t, appImage)
				})

				it("does not restore app metadata", func() {
					h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0, "Platform API < 0.7 restores app metadata")
					output := h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						ctrPath("/layers"),
						analyzeImage,
						h.WithFlags(append(
							dockerSocketMount,
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--env", "CNB_STACK_PATH=/cnb/local-bionic-stack.toml", // /cnb/local-bionic-stack.toml has `io.buildpacks.stacks.bionic` and points to run image `localcompany/stack:bionic` with same stack id
						)...),
						h.WithArgs(
							ctrPath(analyzerPath),
							"-daemon",
							noAuthRegistry.RepoName(appImage)),
					)

					assertNoRestoreOfAppMetadata(t, copyDir, output)
				})

				it("restores app metadata", func() {
					h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore app metadata")
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
							appImage,
						),
					)

					assertLogsAndRestoresAppMetadata(t, copyDir, output)
				})

				when("skip layers is provided", func() {
					it("writes analyzed.toml and does not write buildpack layer metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not accept a -skip-layers flag")
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
								appImage,
							),
						)

						assertAnalyzedMetadata(t, filepath.Join(copyDir, "layers", "analyzed.toml"))
						assertWritesStoreTomlOnly(t, copyDir, output)
					})
				})
			})

			when("cache is provided", func() {
				when("cache image case", func() {
					when("cache image is in a daemon", func() {
						var cacheImage string

						it.Before(func() {
							metadata := minifyMetadata(t, filepath.Join("testdata", "analyzer", "cache_image_metadata.json"), platform.CacheMetadata{})
							cacheImage = "some-cache-image-" + h.RandString(10)

							cmd := exec.Command(
								"docker",
								"build",
								"-t", cacheImage,
								"--build-arg", "fromImage="+containerBaseImage,
								"--build-arg", "metadata="+metadata,
								filepath.Join("testdata", "analyzer", "cache-image"),
							)
							h.Run(t, cmd)
						})

						it.After(func() {
							h.DockerImageRemove(t, cacheImage)
						})

						it("ignores the cache", func() {
							h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read from the cache")

							h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers"),
								analyzeImage,
								h.WithFlags(append(
									dockerSocketMount,
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--env", "CNB_REGISTRY_AUTH="+basicAuth,
								)...),
								h.WithArgs(
									ctrPath(analyzerPath),
									"-daemon",
									"-cache-image", authRegistry.RepoName(cacheImage),
									authRegistry.RepoName("some-image"),
								),
							)

							h.AssertPathDoesNotExist(t, filepath.Join(copyDir, "layers", "some-buildpack-id", "some-layer.sha"))
							h.AssertPathDoesNotExist(t, filepath.Join(copyDir, "layers", "some-buildpack-id", "some-layer.toml"))
						})
					})

					when("cache image is in a registry", func() {
						when("auth registry", func() {
							var authRegCacheImage, cacheAuthConfig string

							it.Before(func() {
								metadata := minifyMetadata(t, filepath.Join("testdata", "analyzer", "cache_image_metadata.json"), platform.CacheMetadata{})
								authRegCacheImage, cacheAuthConfig = buildAuthRegistryImage(
									t,
									"some-cache-image-"+h.RandString(10),
									filepath.Join("testdata", "analyzer", "cache-image"),
									"--build-arg", "fromImage="+containerBaseImage,
									"--build-arg", "metadata="+metadata,
								)
							})

							// Don't attempt to remove the image, as it's stored in the test registry, which is ephemeral.
							// Attempting to remove the image sometimes produces `No such image` flakes.

							when("registry creds are provided in CNB_REGISTRY_AUTH", func() {
								it("restores cache metadata", func() {
									h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read from the cache")
									output := h.DockerRunAndCopy(t,
										containerName,
										copyDir,
										"/layers",
										analyzeImage,
										h.WithFlags(append(
											dockerSocketMount,
											"--network", registryNetwork,
											"--env", "CNB_REGISTRY_AUTH="+cacheAuthConfig,
											"--env", "CNB_PLATFORM_API="+platformAPI,
										)...),
										h.WithArgs(
											ctrPath(analyzerPath),
											"-daemon",
											"-cache-image", authRegCacheImage,
											"some-image",
										),
									)

									assertLogsAndRestoresCacheMetadata(t, copyDir, output)
								})
							})

							when("registry creds are provided in the docker config.json", func() {
								it("restores cache metadata", func() {
									h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read from the cache")
									output := h.DockerRunAndCopy(t,
										containerName,
										copyDir,
										ctrPath("/layers"),
										analyzeImage,
										h.WithFlags(
											"--env", "DOCKER_CONFIG=/docker-config",
											"--network", registryNetwork,
											"--env", "CNB_PLATFORM_API="+platformAPI,
										),
										h.WithArgs(
											ctrPath(analyzerPath),
											"-cache-image",
											authRegCacheImage,
											"some-image",
										),
									)

									assertLogsAndRestoresCacheMetadata(t, copyDir, output)
								})
							})
						})

						when("no auth registry", func() {
							var noAuthRegCacheImage string

							it.Before(func() {
								metadata := minifyMetadata(t, filepath.Join("testdata", "analyzer", "cache_image_metadata.json"), platform.CacheMetadata{})

								imageName := "some-cache-image-" + h.RandString(10)
								buildAuthRegistryImage(
									t,
									imageName,
									filepath.Join("testdata", "analyzer", "cache-image"),
									"--build-arg", "fromImage="+containerBaseImage,
									"--build-arg", "metadata="+metadata,
								)

								noAuthRegCacheImage = noAuthRegistry.RepoName(imageName)
							})

							// Don't attempt to remove the image, as it's stored in the test registry, which is ephemeral.
							// Attempting to remove the image sometimes produces `No such image` flakes.

							it("restores cache metadata", func() {
								h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read from the cache")
								output := h.DockerRunAndCopy(t,
									containerName,
									copyDir,
									ctrPath("/layers"),
									analyzeImage,
									h.WithFlags(append(
										dockerSocketMount,
										"--network", registryNetwork,
										"--env", "CNB_PLATFORM_API="+platformAPI,
									)...),
									h.WithArgs(
										ctrPath(analyzerPath),
										"-daemon",
										"-cache-image",
										noAuthRegCacheImage,
										"some-image",
									),
								)

								assertLogsAndRestoresCacheMetadata(t, copyDir, output)
							})
						})
					})
				})

				when("cache directory case", func() {
					it("restores cache metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read from the cache")
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
							h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read from the cache")
							h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

							cacheVolume := h.SeedDockerVolume(t, cacheFixtureDir)
							defer h.DockerVolumeRemove(t, cacheVolume)

							output := h.DockerRun(t,
								analyzeImage,
								h.WithFlags(append(
									dockerSocketMount,
									"--volume", cacheVolume+":/cache",
									"--env", "CNB_PLATFORM_API="+platformAPI,
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
							h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read from the cache")

							cacheVolume := h.SeedDockerVolume(t, cacheFixtureDir)
							defer h.DockerVolumeRemove(t, cacheVolume)

							output := h.DockerRun(t,
								analyzeImage,
								h.WithFlags(append(
									dockerSocketMount,
									"--volume", cacheVolume+":/cache",
									"--env", "CNB_PLATFORM_API="+platformAPI,
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
				h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					ctrPath("/layers/analyzed.toml"),
					analyzeImage,
					h.WithFlags(
						"--network", registryNetwork,
						"--env", "CNB_PLATFORM_API="+platformAPI,
					),
					h.WithArgs(ctrPath(analyzerPath), noAuthRegistry.RepoName("some-image")),
				)

				assertAnalyzedMetadata(t, filepath.Join(copyDir, "analyzed.toml"))
			})

			when("app image exists", func() {
				when("auth registry", func() {
					var authRegAppImage, appAuthConfig string

					it.Before(func() {
						metadata := minifyMetadata(t, filepath.Join("testdata", "analyzer", "app_image_metadata.json"), platform.LayersMetadata{})
						authRegAppImage, appAuthConfig = buildAuthRegistryImage(
							t,
							"some-app-image-"+h.RandString(10),
							filepath.Join("testdata", "analyzer", "app-image"),
							"--build-arg", "fromImage="+containerBaseImage,
							"--build-arg", "metadata="+metadata,
						)
					})

					// Don't attempt to remove the image, as it's stored in the test registry, which is ephemeral.
					// Attempting to remove the image sometimes produces `No such image` flakes.

					when("registry creds are provided in CNB_REGISTRY_AUTH", func() {
						it("restores app metadata", func() {
							h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read app layer metadata")
							output := h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers"),
								analyzeImage,
								h.WithFlags(
									"--env", "CNB_REGISTRY_AUTH="+appAuthConfig,
									"--network", registryNetwork,
									"--env", "CNB_PLATFORM_API="+platformAPI,
								),
								h.WithArgs(
									ctrPath(analyzerPath),
									authRegAppImage,
								),
							)

							assertLogsAndRestoresAppMetadata(t, copyDir, output)
						})
					})

					when("registry creds are provided in the docker config.json", func() {
						it("restores app metadata", func() {
							h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read app layer metadata")
							output := h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers"),
								analyzeImage,
								h.WithFlags(
									"--env", "DOCKER_CONFIG=/docker-config",
									"--network", registryNetwork,
									"--env", "CNB_PLATFORM_API="+platformAPI,
								),
								h.WithArgs(
									ctrPath(analyzerPath),
									authRegAppImage,
								),
							)

							assertLogsAndRestoresAppMetadata(t, copyDir, output)
						})
					})

					when("skip layers is provided", func() {
						it("writes analyzed.toml and does not write buildpack layer metadata", func() {
							h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not accept a -skip-layers flag")
							output := h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers"),
								analyzeImage,
								h.WithFlags(
									"--network", registryNetwork,
									"--env", "CNB_REGISTRY_AUTH="+appAuthConfig,
									"--env", "CNB_PLATFORM_API="+platformAPI,
								),
								h.WithArgs(
									ctrPath(analyzerPath),
									"-skip-layers",
									authRegAppImage,
								),
							)

							assertAnalyzedMetadata(t, filepath.Join(copyDir, "layers", "analyzed.toml"))
							assertWritesStoreTomlOnly(t, copyDir, output)
						})
					})
				})

				when("no auth registry", func() {
					var noAuthRegAppImage string

					it.Before(func() {
						metadata := minifyMetadata(t, filepath.Join("testdata", "analyzer", "app_image_metadata.json"), platform.LayersMetadata{})

						imageName := "some-app-image-" + h.RandString(10)
						buildAuthRegistryImage(
							t,
							imageName,
							filepath.Join("testdata", "analyzer", "app-image"),
							"--build-arg", "fromImage="+containerBaseImage,
							"--build-arg", "metadata="+metadata,
						)

						noAuthRegAppImage = noAuthRegistry.RepoName(imageName)
					})

					// Don't attempt to remove the image, as it's stored in the test registry, which is ephemeral.
					// Attempting to remove the image sometimes produces `No such image` flakes.

					it("restores app metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read app layer metadata")
						output := h.DockerRunAndCopy(t,
							containerName,
							copyDir,
							ctrPath("/layers"),
							analyzeImage,
							h.WithFlags(
								"--network", registryNetwork,
								"--env", "CNB_PLATFORM_API="+platformAPI,
							),
							h.WithArgs(
								ctrPath(analyzerPath),
								noAuthRegAppImage,
							),
						)

						assertLogsAndRestoresAppMetadata(t, copyDir, output)
					})

					when("skip layers is provided", func() {
						it("writes analyzed.toml and does not write buildpack layer metadata", func() {
							h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not accept a -skip-layers flag")
							output := h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers"),
								analyzeImage,
								h.WithFlags(
									"--network", registryNetwork,
									"--env", "CNB_PLATFORM_API="+platformAPI,
								),
								h.WithArgs(
									ctrPath(analyzerPath),
									"-skip-layers",
									noAuthRegAppImage,
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
					h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0, "Platform API < 0.7 does not support -previous-image")
				})

				when("auth registry", func() {
					var authRegAppImage, authRegAppOtherImage, appAuthConfig string

					it.Before(func() {
						metadata := minifyMetadata(t, filepath.Join("testdata", "analyzer", "app_image_metadata.json"), platform.LayersMetadata{})
						authRegAppImage, appAuthConfig = buildAuthRegistryImage(
							t,
							"some-app-image-"+h.RandString(10),
							filepath.Join("testdata", "analyzer", "app-image"),
							"--build-arg", "fromImage="+containerBaseImage,
							"--build-arg", "metadata="+metadata,
						)

						authRegAppOtherImage, appAuthConfig = buildAuthRegistryImage(
							t,
							"some-app-image-"+h.RandString(10),
							filepath.Join("testdata", "analyzer", "app-image"),
							"--build-arg", "fromImage="+containerBaseImage,
							"--build-arg", "metadata="+metadata,
						)
					})

					when("the destination image does not exist", func() {
						it("writes analyzed.toml with previous image identifier", func() {
							execArgs := []string{
								ctrPath(analyzerPath),
								"-previous-image", authRegAppImage,
								"some-fake-image",
							}

							h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers/analyzed.toml"),
								analyzeImage,
								h.WithFlags(
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--env", "CNB_REGISTRY_AUTH="+appAuthConfig,
									"--network", registryNetwork,
								),
								h.WithArgs(execArgs...),
							)

							md := getAnalyzedMetadata(t, filepath.Join(copyDir, "analyzed.toml"))
							h.AssertStringContains(t, md.Image.Reference, authRegAppImage)
						})
					})

					when("the destination image exists", func() {
						it("writes analyzed.toml with previous image identifier", func() {
							execArgs := []string{
								ctrPath(analyzerPath),
								"-previous-image", authRegAppImage,
								authRegAppOtherImage,
							}

							h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers/analyzed.toml"),
								analyzeImage,
								h.WithFlags(
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--env", "CNB_REGISTRY_AUTH="+appAuthConfig,
									"--network", registryNetwork,
								),
								h.WithArgs(execArgs...),
							)

							md := getAnalyzedMetadata(t, filepath.Join(copyDir, "analyzed.toml"))
							h.AssertStringContains(t, md.Image.Reference, authRegAppImage)
						})
					})
				})
			})

			when("cache is provided", func() {
				when("cache image case", func() {
					when("auth registry", func() {
						var authRegCacheImage, cacheAuthConfig string

						it.Before(func() {
							metadata := minifyMetadata(t, filepath.Join("testdata", "analyzer", "cache_image_metadata.json"), platform.CacheMetadata{})
							authRegCacheImage, cacheAuthConfig = buildAuthRegistryImage(
								t,
								"some-cache-image-"+h.RandString(10),
								filepath.Join("testdata", "analyzer", "cache-image"),
								"--build-arg", "fromImage="+containerBaseImage,
								"--build-arg", "metadata="+metadata,
							)
						})

						// Don't attempt to remove the image, as it's stored in the test registry, which is ephemeral.
						// Attempting to remove the image sometimes produces `No such image` flakes.

						when("registry creds are provided in CNB_REGISTRY_AUTH", func() {
							it("restores cache metadata", func() {
								h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read from the cache")
								output := h.DockerRunAndCopy(t,
									containerName,
									copyDir,
									ctrPath("/layers"),
									analyzeImage,
									h.WithFlags(
										"--env", "CNB_REGISTRY_AUTH="+cacheAuthConfig,
										"--network", registryNetwork,
										"--env", "CNB_PLATFORM_API="+platformAPI,
									),
									h.WithArgs(
										ctrPath(analyzerPath),
										"-cache-image", authRegCacheImage,
										"some-image",
									),
								)

								assertLogsAndRestoresCacheMetadata(t, copyDir, output)
							})
						})

						when("registry creds are provided in the docker config.json", func() {
							it("restores cache metadata", func() {
								h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read from the cache")
								output := h.DockerRunAndCopy(t,
									containerName,
									copyDir,
									ctrPath("/layers"),
									analyzeImage,
									h.WithFlags(
										"--env", "DOCKER_CONFIG=/docker-config",
										"--network", registryNetwork,
										"--env", "CNB_PLATFORM_API="+platformAPI,
									),
									h.WithArgs(
										ctrPath(analyzerPath),
										"-cache-image",
										authRegCacheImage,
										"some-image",
									),
								)

								assertLogsAndRestoresCacheMetadata(t, copyDir, output)
							})
						})
					})

					when("no auth registry", func() {
						var noAuthRegCacheImage string

						it.Before(func() {
							metadata := minifyMetadata(t, filepath.Join("testdata", "analyzer", "cache_image_metadata.json"), platform.CacheMetadata{})

							imageName := "some-cache-image-" + h.RandString(10)
							buildAuthRegistryImage(
								t,
								imageName,
								filepath.Join("testdata", "analyzer", "cache-image"),
								"--build-arg", "fromImage="+containerBaseImage,
								"--build-arg", "metadata="+metadata,
							)

							noAuthRegCacheImage = noAuthRegistry.RepoName(imageName)
						})

						// Don't attempt to remove the image, as it's stored in the test registry, which is ephemeral.
						// Attempting to remove the image sometimes produces `No such image` flakes.

						it("throw read/write error", func() {
							h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read from the cache")
							cmd := exec.Command(
								"docker", "run", "--rm",
								"--network", registryNetwork,
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--name", containerName,
								analyzeImage,
								ctrPath(analyzerPath),
								"-cache-image",
								noAuthRegCacheImage,
								"some-image",
							) // #nosec G204
							output, err := cmd.CombinedOutput()

							h.AssertNotNil(t, err)
							expected := "failed to : read/write image "+noAuthRegCacheImage+" from/to the registry"
							h.AssertStringContains(t, string(output), expected)
						})
					})
				})

				when("cache directory case", func() {
					it("restores cache metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not read from the cache")
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
		})

		when("layers path is provided", func() {
			it("uses the group path at the working directory and writes analyzed.toml at the working directory", func() {
				h.SkipIf(t,
					api.MustParse(platformAPI).Compare(api.MustParse("0.5")) >= 0,
					"Platform API 0.5 and 0.6 read and write to the provided layers directory; Platform 0.7+ does not accept a -cache-dir flag",
				)

				otherLayersDir := filepath.Join(copyDir, "other-layers")
				layersDir := filepath.Join(copyDir, "layers")

				// The working directory is set to /layers in the Dockerfile
				h.DockerSeedRunAndCopy(t,
					containerName,
					cacheFixtureDir, ctrPath("/cache"),
					otherLayersDir, ctrPath("/other-layers"),
					analyzeImage,
					h.WithFlags(
						"--env", "CNB_PLATFORM_API="+platformAPI,
					),
					h.WithArgs(
						ctrPath(analyzerPath),
						"-layers", ctrPath("/other-layers"),
						"-cache-dir", ctrPath("/cache"), // use a cache so that we can observe the effect of group.toml (since we don't have a previous image)
						"some-image",
					),
				)
				h.AssertPathExists(t, filepath.Join(otherLayersDir, "some-buildpack-id")) // some-buildpack-id is found in the working directory: /layers/group.toml

				h.DockerCopyOut(t, containerName, ctrPath("/layers"), layersDir) // analyzed.toml is written at the working directory: /layers
				assertAnalyzedMetadata(t, filepath.Join(layersDir, "analyzed.toml"))
			})

			it("uses the group path at the layers path and writes analyzed.toml at the layers path", func() {
				h.SkipIf(t,
					api.MustParse(platformAPI).Compare(api.MustParse("0.5")) != 0 && api.MustParse(platformAPI).Compare(api.MustParse("0.6")) != 0,
					"Platform API < 0.5 reads and writes to the working directory; Platform 0.7+ does not accept a -cache-dir flag",
				)

				h.DockerSeedRunAndCopy(t,
					containerName,
					cacheFixtureDir, ctrPath("/cache"),
					copyDir, ctrPath("/some-other-layers"),
					analyzeImage,
					h.WithFlags(
						"--env", "CNB_PLATFORM_API="+platformAPI,
					),
					h.WithArgs(
						ctrPath(analyzerPath),
						"-layers", ctrPath("/some-other-layers"),
						"-cache-dir", ctrPath("/cache"), // use a cache so that we can observe the effect of group.toml (since we don't have a previous image)
						"some-image",
					),
				)
				h.AssertPathExists(t, filepath.Join(copyDir, "some-other-layers", "another-buildpack-id")) // another-buildpack-id is found in the provided -layers directory: /some-other-layers/group.toml

				assertAnalyzedMetadata(t, filepath.Join(copyDir, "some-other-layers", "analyzed.toml")) // analyzed.toml is written at the provided -layers directory: /some-other-layers
			})
		})

		when("validating stack", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0, "Platform API < 0.7 does not validate stack")
			})

			when("stack metadata is present", func() {
				when("stacks match", func() {
					it("passes validation", func() {
						execArgs := []string{ctrPath(analyzerPath), noAuthRegistry.RepoName("some-image")}
						h.DockerRun(t,
							analyzeImage, // /cnb/stack.toml has `io.buildpacks.stacks.bionic` and points to run image `company/stack:bionic` with same stack id
							h.WithFlags(
								"--network", registryNetwork,
								"--env", "CNB_PLATFORM_API="+platformAPI,
							),
							h.WithArgs(execArgs...),
						)
					})
				})

				when("CNB_RUN_IMAGE is present", func() {
					it("uses CNB_RUN_IMAGE for validation", func() {
						execArgs := []string{ctrPath(analyzerPath), noAuthRegistry.RepoName("some-image")}

						h.DockerRun(t,
							analyzeImage,
							h.WithFlags(
								"--network", registryNetwork,
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_STACK_PATH=/cnb/mismatch-stack.toml", // /cnb/mismatch-stack.toml points to run image `company/stack:centos`
								"--env", "CNB_RUN_IMAGE="+noAuthRegistry.RepoName("company/stack:bionic"),
							),
							h.WithArgs(execArgs...),
						)
					})
				})

				when("stack metadata file is invalid", func() {
					it("fails validation", func() {
						cmd := exec.Command(
							"docker", "run", "--rm",
							"--network", registryNetwork,
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--env", "CNB_STACK_PATH=/cnb/bad-stack.toml",
							analyzeImage,
							ctrPath(analyzerPath),
							noAuthRegistry.RepoName("some-image"),
						) // #nosec G204
						output, err := cmd.CombinedOutput()

						h.AssertNotNil(t, err)
						expected := "get stack metadata"
						h.AssertStringContains(t, string(output), expected)
					})
				})

				when("run image inaccessible", func() {
					it("fails validation", func() {
						cmd := exec.Command(
							"docker", "run", "--rm",
							"--network", registryNetwork,
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--env", "CNB_RUN_IMAGE=fake.example.com/company/example:20",
							analyzeImage,
							ctrPath(analyzerPath),
							noAuthRegistry.RepoName("some-image"),
						) // #nosec G204
						output, err := cmd.CombinedOutput()

						h.AssertNotNil(t, err)
						expected := "read image fake.example.com/company/example:20 from the registry"
						h.AssertStringContains(t, string(output), expected)
					})
				})

				when("run image has mirrors", func() {
					it("uses expected mirror for run-image", func() {
						execArgs := []string{ctrPath(analyzerPath), noAuthRegistry.RepoName("apprepo/myapp")} // image located on same registry as mirror

						h.DockerRunAndCopy(t,
							containerName,
							copyDir,
							ctrPath("/layers/analyzed.toml"),
							analyzeImage,
							h.WithFlags(
								"--network", registryNetwork,
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_STACK_PATH=/cnb/run-mirror-stack.toml", // /cnb/run-mirror-stack.toml points to run image on gcr.io and mirror on test registry
							),
							h.WithArgs(execArgs...),
						)
					})
				})

				when("daemon case", func() {
					when("stacks match", func() {
						it("passes validation", func() {
							execArgs := []string{ctrPath(analyzerPath), "-daemon", noAuthRegistry.RepoName("some-image")}

							h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers/analyzed.toml"),
								analyzeImage,
								h.WithFlags(append(
									dockerSocketMount,
									"--network", registryNetwork,
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--env", "CNB_STACK_PATH=/cnb/local-bionic-stack.toml", // /cnb/local-bionic-stack.toml has `io.buildpacks.stacks.bionic` and points to run image `localcompany/stack:bionic` with same stack id
								)...),
								h.WithArgs(execArgs...),
							)
						})
					})
				})
			})

			when("stack metadata is not present", func() {
				when("CNB_RUN_IMAGE and CNB_STACK_ID are set", func() {
					it("passes validation", func() {
						execArgs := []string{ctrPath(analyzerPath), noAuthRegistry.RepoName("some-image")}

						h.DockerRunAndCopy(t,
							containerName,
							copyDir,
							ctrPath("/layers/analyzed.toml"),
							analyzeImage,
							h.WithFlags(
								"--network", registryNetwork,
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_STACK_PATH=/cnb/file-does-not-exist.toml",
								"--env", "CNB_RUN_IMAGE="+noAuthRegistry.RepoName("company/stack:bionic"),
								"--env", "CNB_STACK_ID=io.buildpacks.stacks.bionic",
							),
							h.WithArgs(execArgs...),
						)
					})
				})

				when("run image and stack id are not provided as arguments or in the environment", func() {
					it("fails validation", func() {
						cmd := exec.Command(
							"docker", "run", "--rm",
							"--network", registryNetwork,
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--env", "CNB_STACK_PATH=/cnb/file-does-not-exist.toml",
							analyzeImage,
							ctrPath(analyzerPath),
							noAuthRegistry.RepoName("some-image"),
						) // #nosec G204
						output, err := cmd.CombinedOutput()

						h.AssertNotNil(t, err)
						expected := "a run image must be specified when there is no stack metadata available"
						h.AssertStringContains(t, string(output), expected)
					})
				})
			})
		})
	}
}

func minifyMetadata(t *testing.T, path string, metadataStruct interface{}) string {
	metadata, err := ioutil.ReadFile(path)
	h.AssertNil(t, err)

	// Unmarshal and marshal to strip unnecessary whitespace
	h.AssertNil(t, json.Unmarshal(metadata, &metadataStruct))
	flatMetadata, err := json.Marshal(metadataStruct)
	h.AssertNil(t, err)

	return string(flatMetadata)
}

func buildAuthRegistryImage(t *testing.T, repoName, context string, buildArgs ...string) (string, string) {
	// Build image
	regRepoName := authRegistry.RepoName(repoName)
	h.DockerBuild(t, regRepoName, context, h.WithArgs(buildArgs...))

	// Push image
	h.AssertNil(t, h.PushImage(h.DockerCli(t), regRepoName, authRegistry.EncodedLabeledAuth()))

	// Setup auth
	authConfig, err := auth.BuildEnvVar(authn.DefaultKeychain, regRepoName)
	h.AssertNil(t, err)

	return regRepoName, authConfig
}

func assertAnalyzedMetadata(t *testing.T, path string) {
	contents, _ := ioutil.ReadFile(path)
	h.AssertEq(t, len(contents) > 0, true)

	var analyzedMd platform.AnalyzedMetadata
	_, err := toml.Decode(string(contents), &analyzedMd)
	h.AssertNil(t, err)
}

func getAnalyzedMetadata(t *testing.T, path string) *platform.AnalyzedMetadata {
	contents, _ := ioutil.ReadFile(path)
	h.AssertEq(t, len(contents) > 0, true)

	var analyzedMd platform.AnalyzedMetadata
	_, err := toml.Decode(string(contents), &analyzedMd)
	h.AssertNil(t, err)

	return &analyzedMd
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

func getBasicAuth() string {
	return fmt.Sprintf("{\"%s\": \"Basic %s\"}", authRegistry.Host+":"+authRegistry.Port, authRegistry.BasicAuth())
}
