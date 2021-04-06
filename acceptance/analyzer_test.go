package acceptance

import (
	"context"
	"encoding/json"
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
	ih "github.com/buildpacks/imgutil/testhelpers"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

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
	registry             *ih.DockerRegistry
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

	regVolumeID := "test-registry-volume-" + h.RandString(10)
	registry = ih.NewDockerRegistry(ih.WithAuth(dockerConfigDir), ih.WithSharedStorageVolume(regVolumeID))
	registry.Start(t)
	defer registry.Stop(t)

	noAuthRegistry = ih.NewDockerRegistry(ih.WithSharedStorageVolume(regVolumeID))
	noAuthRegistry.Start(t)
	defer noAuthRegistry.Stop(t)

	// if registry is listening on localhost, use host networking to allow containers to reach it
	registryNetwork = "default"
	if registry.Host == "localhost" {
		registryNetwork = "host"
	}

	os.Setenv("DOCKER_CONFIG", registry.DockerDirectory)
	// Copy docker config directory to analyze-image container
	targetDockerConfig := filepath.Join("testdata", "analyzer", "analyze-image", "container", "docker-config")
	h.AssertNil(t, os.RemoveAll(filepath.Join(targetDockerConfig, "config.json")))
	h.RecursiveCopy(t, registry.DockerDirectory, targetDockerConfig)

	// Setup test container

	h.MakeAndCopyLifecycle(t, daemonOS, analyzerBinaryDir)
	h.DockerBuild(t,
		analyzeImage,
		analyzeDockerContext,
		h.WithFlags("-f", filepath.Join(analyzeDockerContext, dockerfileName)),
	)
	defer h.DockerImageRemove(t, analyzeImage)

	spec.Run(t, "acceptance-analyzer", testAnalyzer, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testAnalyzer(t *testing.T, when spec.G, it spec.S) {
	var copyDir, containerName, cacheVolume string

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
		if h.DockerVolumeExists(t, cacheVolume) {
			h.DockerVolumeRemove(t, cacheVolume)
		}
		os.RemoveAll(copyDir)
	})

	when("called without an app image", func() {
		it("errors", func() {
			cmd := exec.Command(
				"docker", "run", "--rm",
				"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				analyzeImage,
				ctrPath(analyzerPath),
			) // #nosec G204
			output, err := cmd.CombinedOutput()

			h.AssertNotNil(t, err)
			expected := "failed to parse arguments: received 0 arguments, but expected 1"
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("cache image tag and cache directory are both blank", func() {
		it("warns", func() {
			output := h.DockerRun(t,
				analyzeImage,
				h.WithFlags("--env", "CNB_PLATFORM_API="+latestPlatformAPI),
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
				h.WithFlags("--env", "CNB_PLATFORM_API="+latestPlatformAPI),
				h.WithBash(fmt.Sprintf("chown -R 9999:9999 /layers; chmod -R 775 /layers; %s some-image; ls -al /layers", analyzerPath)),
			)

			h.AssertMatch(t, output, "2222 3333 .+ \\.")
			h.AssertMatch(t, output, "2222 3333 .+ group.toml")
		})
	})

	when("group path is provided", func() {
		it("uses the provided group path", func() {
			h.DockerSeedRunAndCopy(t,
				containerName,
				cacheFixtureDir, ctrPath("/cache"),
				copyDir, ctrPath("/layers"),
				analyzeImage,
				h.WithFlags(
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
			h.DockerRunAndCopy(t,
				containerName,
				copyDir,
				ctrPath("/some-dir/some-analyzed.toml"),
				analyzeImage,
				h.WithFlags("--env", "CNB_PLATFORM_API="+latestPlatformAPI),
				h.WithArgs(
					ctrPath(analyzerPath),
					"-analyzed", ctrPath("/some-dir/some-analyzed.toml"),
					"some-image",
				),
			)

			assertAnalyzedMetadata(t, filepath.Join(copyDir, "some-analyzed.toml"))
		})
	})

	when("daemon case", func() {
		it("writes analyzed.toml", func() {
			h.DockerRunAndCopy(t,
				containerName,
				copyDir,
				ctrPath("/layers/analyzed.toml"),
				analyzeImage,
				h.WithFlags(append(
					dockerSocketMount,
					"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
				)...),
				h.WithArgs(
					ctrPath(analyzerPath),
					"-daemon",
					"some-image",
				),
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

			it("restores app metadata", func() {
				output := h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					ctrPath("/layers"),
					analyzeImage,
					h.WithFlags(append(
						dockerSocketMount,
						"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
					output := h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						ctrPath("/layers"),
						analyzeImage,
						h.WithFlags(append(
							dockerSocketMount,
							"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
						h.DockerRunAndCopy(t,
							containerName,
							copyDir,
							ctrPath("/layers"),
							analyzeImage,
							h.WithFlags(append(
								dockerSocketMount,
								"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
							)...),
							h.WithArgs(
								ctrPath(analyzerPath),
								"-daemon",
								"-cache-image", cacheImage,
								"some-image",
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

						it.After(func() {
							h.DockerImageRemove(t, authRegCacheImage)
						})

						when("registry creds are provided in CNB_REGISTRY_AUTH", func() {
							it("restores cache metadata", func() {
								output := h.DockerRunAndCopy(t,
									containerName,
									copyDir,
									"/layers",
									analyzeImage,
									h.WithFlags(append(
										dockerSocketMount,
										"--network", registryNetwork,
										"--env", "CNB_REGISTRY_AUTH="+cacheAuthConfig,
										"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
								output := h.DockerRunAndCopy(t,
									containerName,
									copyDir,
									ctrPath("/layers"),
									analyzeImage,
									h.WithFlags(
										"--env", "DOCKER_CONFIG=/docker-config",
										"--network", registryNetwork,
										"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
						var authRegCacheImage, noAuthRegCacheImage string

						it.Before(func() {
							metadata := minifyMetadata(t, filepath.Join("testdata", "analyzer", "cache_image_metadata.json"), platform.CacheMetadata{})

							imageName := "some-cache-image-" + h.RandString(10)
							authRegCacheImage, _ = buildAuthRegistryImage(
								t,
								imageName,
								filepath.Join("testdata", "analyzer", "cache-image"),
								"--build-arg", "fromImage="+containerBaseImage,
								"--build-arg", "metadata="+metadata,
							)

							noAuthRegCacheImage = noAuthRegistry.RepoName(imageName)
						})

						it.After(func() {
							h.DockerImageRemove(t, authRegCacheImage)
						})

						it("restores cache metadata", func() {
							output := h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers"),
								analyzeImage,
								h.WithFlags(
									"--network", registryNetwork,
									"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
								),
								h.WithArgs(
									ctrPath(analyzerPath),
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
					output := h.DockerSeedRunAndCopy(t,
						containerName,
						cacheFixtureDir, ctrPath("/cache"),
						copyDir, ctrPath("/layers"),
						analyzeImage,
						h.WithFlags(append(
							dockerSocketMount,
							"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
						h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

						cacheVolume := h.SeedDockerVolume(t, cacheFixtureDir)
						defer h.DockerVolumeRemove(t, cacheVolume)

						output := h.DockerRun(t,
							analyzeImage,
							h.WithFlags(append(
								dockerSocketMount,
								"--volume", cacheVolume+":/cache",
								"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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

						cacheVolume := h.SeedDockerVolume(t, cacheFixtureDir)
						defer h.DockerVolumeRemove(t, cacheVolume)

						output := h.DockerRun(t,
							analyzeImage,
							h.WithFlags(append(
								dockerSocketMount,
								"--volume", cacheVolume+":/cache",
								"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
				h.WithArgs(
					ctrPath(analyzerPath),
					"some-image",
				),
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

				it.After(func() {
					h.DockerImageRemove(t, authRegAppImage)
				})

				when("registry creds are provided in CNB_REGISTRY_AUTH", func() {
					it("restores app metadata", func() {
						output := h.DockerRunAndCopy(t,
							containerName,
							copyDir,
							ctrPath("/layers"),
							analyzeImage,
							h.WithFlags(
								"--env", "CNB_REGISTRY_AUTH="+appAuthConfig,
								"--network", registryNetwork,
								"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
						output := h.DockerRunAndCopy(t,
							containerName,
							copyDir,
							ctrPath("/layers"),
							analyzeImage,
							h.WithFlags(
								"--env", "DOCKER_CONFIG=/docker-config",
								"--network", registryNetwork,
								"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
						output := h.DockerRunAndCopy(t,
							containerName,
							copyDir,
							ctrPath("/layers"),
							analyzeImage,
							h.WithFlags(
								"--network", registryNetwork,
								"--env", "CNB_REGISTRY_AUTH="+appAuthConfig,
								"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
				var authRegAppImage, noAuthRegAppImage string

				it.Before(func() {
					metadata := minifyMetadata(t, filepath.Join("testdata", "analyzer", "app_image_metadata.json"), platform.LayersMetadata{})

					imageName := "some-app-image-" + h.RandString(10)
					authRegAppImage, _ = buildAuthRegistryImage(
						t,
						imageName,
						filepath.Join("testdata", "analyzer", "app-image"),
						"--build-arg", "fromImage="+containerBaseImage,
						"--build-arg", "metadata="+metadata,
					)

					noAuthRegAppImage = noAuthRegistry.RepoName(imageName)
				})

				it.After(func() {
					h.DockerImageRemove(t, authRegAppImage)
				})

				it("restores app metadata", func() {
					output := h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						ctrPath("/layers"),
						analyzeImage,
						h.WithFlags(
							"--network", registryNetwork,
							"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
						output := h.DockerRunAndCopy(t,
							containerName,
							copyDir,
							ctrPath("/layers"),
							analyzeImage,
							h.WithFlags(
								"--network", registryNetwork,
								"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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

					it.After(func() {
						h.DockerImageRemove(t, authRegCacheImage)
					})

					when("registry creds are provided in CNB_REGISTRY_AUTH", func() {
						it("restores cache metadata", func() {
							output := h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers"),
								analyzeImage,
								h.WithFlags(
									"--env", "CNB_REGISTRY_AUTH="+cacheAuthConfig,
									"--network", registryNetwork,
									"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
							output := h.DockerRunAndCopy(t,
								containerName,
								copyDir,
								ctrPath("/layers"),
								analyzeImage,
								h.WithFlags(
									"--env", "DOCKER_CONFIG=/docker-config",
									"--network", registryNetwork,
									"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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
					var authRegCacheImage, noAuthRegCacheImage string

					it.Before(func() {
						metadata := minifyMetadata(t, filepath.Join("testdata", "analyzer", "cache_image_metadata.json"), platform.CacheMetadata{})

						imageName := "some-cache-image-" + h.RandString(10)
						authRegCacheImage, _ = buildAuthRegistryImage(
							t,
							imageName,
							filepath.Join("testdata", "analyzer", "cache-image"),
							"--build-arg", "fromImage="+containerBaseImage,
							"--build-arg", "metadata="+metadata,
						)

						noAuthRegCacheImage = noAuthRegistry.RepoName(imageName)
					})

					it.After(func() {
						h.DockerImageRemove(t, authRegCacheImage)
					})

					it("restores cache metadata", func() {
						output := h.DockerRunAndCopy(t,
							containerName,
							copyDir,
							ctrPath("/layers"),
							analyzeImage,
							h.WithFlags(
								"--network", registryNetwork,
								"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
							),
							h.WithArgs(
								ctrPath(analyzerPath),
								"-cache-image", noAuthRegCacheImage,
								"some-image",
							),
						)

						assertLogsAndRestoresCacheMetadata(t, copyDir, output)
					})
				})
			})

			when("cache directory case", func() {
				it("restores cache metadata", func() {
					output := h.DockerSeedRunAndCopy(t,
						containerName,
						cacheFixtureDir, ctrPath("/cache"),
						copyDir, ctrPath("/layers"),
						analyzeImage,
						h.WithFlags(
							"--env", "CNB_PLATFORM_API="+latestPlatformAPI,
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

	when("Platform API < 0.5", func() {
		when("layers path is provided", func() {
			it("uses the group path at the working directory and writes analyzed.toml at the working directory", func() {
				otherLayersDir := filepath.Join(copyDir, "other-layers")
				layersDir := filepath.Join(copyDir, "layers")

				// The working directory is set to /layers in the Dockerfile
				h.DockerSeedRunAndCopy(t,
					containerName,
					cacheFixtureDir, ctrPath("/cache"),
					otherLayersDir, ctrPath("/other-layers"),
					analyzeImage,
					h.WithFlags(
						"--env", "CNB_PLATFORM_API=0.4",
					),
					h.WithArgs(
						ctrPath(analyzerPath),
						"-layers", ctrPath("/other-layers"),
						"-cache-dir", ctrPath("/cache"), // use a cache so that we can observe the effect of group.toml on /some-other-layers (since we don't have a previous image)
						"some-image",
					),
				)
				h.AssertPathExists(t, filepath.Join(otherLayersDir, "some-buildpack-id"))

				h.DockerCopyOut(t, containerName, ctrPath("/layers"), layersDir)
				assertAnalyzedMetadata(t, filepath.Join(layersDir, "analyzed.toml"))
			})
		})
	})

	when("Platform API = 0.5", func() {
		when("layers path is provided", func() {
			it("uses the group path at the layers path and writes analyzed.toml at the layers path", func() {
				h.DockerSeedRunAndCopy(t,
					containerName,
					cacheFixtureDir, ctrPath("/cache"),
					copyDir, ctrPath("/some-other-layers"),
					analyzeImage,
					h.WithFlags(
						"--env", "CNB_PLATFORM_API=0.5",
					),
					h.WithArgs(
						ctrPath(analyzerPath),
						"-layers", ctrPath("/some-other-layers"),
						"-cache-dir", ctrPath("/cache"), // use a cache so that we can observe the effect of group.toml on /some-other-layers (since we don't have a previous image)
						"some-image",
					),
				)

				assertAnalyzedMetadata(t, filepath.Join(copyDir, "some-other-layers", "analyzed.toml"))
				h.AssertPathExists(t, filepath.Join(copyDir, "some-other-layers", "another-buildpack-id"))
			})
		})
	})
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
	regRepoName := registry.RepoName(repoName)
	h.DockerBuild(t, regRepoName, context, h.WithArgs(buildArgs...))

	// Push image
	h.AssertNil(t, h.PushImage(h.DockerCli(t), regRepoName, registry.EncodedLabeledAuth()))

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

func assertLogsAndRestoresAppMetadata(t *testing.T, dir, output string) {
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
		h.AssertPathExists(t, filepath.Join(dir, "layers", "some-buildpack-id", filename))
	}
	layerNames := []string{
		"launch-build-cache-layer",
		"launch-cache-layer",
		"launch-layer",
	}
	for _, layerName := range layerNames {
		h.AssertStringContains(t, output, fmt.Sprintf("Restoring metadata for \"some-buildpack-id:%s\"", layerName))
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
