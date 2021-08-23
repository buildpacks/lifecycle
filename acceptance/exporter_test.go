// +build acceptance

package acceptance

import (
	"context"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	ih "github.com/buildpacks/imgutil/testhelpers"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	exporterBinaryDir   = filepath.Join("testdata", "exporter", "container", "cnb", "lifecycle")
	exportDockerContext = filepath.Join("testdata", "exporter")
	exportImage         = "lifecycle/acceptance/exporter"
	exporterPath        = "/cnb/lifecycle/exporter"
)

func TestExporter(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "Exporter acceptance tests are not yet supported on Windows")

	rand.Seed(time.Now().UTC().UnixNano())

	// TODO: make helper functions to avoid duplication
	info, err := h.DockerCli(t).Info(context.TODO())
	h.AssertNil(t, err)
	daemonOS = info.OSType
	daemonArch = info.Architecture
	if daemonArch == "x86_64" {
		daemonArch = "amd64"
	}
	if daemonArch == "aarch64" { // TODO: propagate everywhere
		daemonArch = "arm64"
	}

	// Setup registry

	dockerConfigDir, err := ioutil.TempDir("", "test.docker.config.dir")
	h.AssertNil(t, err)
	defer os.RemoveAll(dockerConfigDir)

	sharedRegHandler := registry.New(registry.Logger(log.New(ioutil.Discard, "", log.Lshortfile)))
	testRegistry = ih.NewDockerRegistry(ih.WithAuth(dockerConfigDir), ih.WithSharedHandler(sharedRegHandler),
		ih.WithImagePrivileges())

	testRegistry.Start(t)
	defer testRegistry.Stop(t)

	// if registry is listening on localhost, use host networking to allow containers to reach it
	registryNetwork = "default"
	if testRegistry.Host == "localhost" {
		registryNetwork = "host"
	}

	os.Setenv("DOCKER_CONFIG", testRegistry.DockerDirectory)

	// Copy docker config directory to test container
	targetDockerConfig := filepath.Join("testdata", "exporter", "container", "docker-config")
	h.AssertNil(t, os.RemoveAll(filepath.Join(targetDockerConfig, "config.json")))
	h.RecursiveCopy(t, testRegistry.DockerDirectory, targetDockerConfig)

	// end TODO

	// Setup fixtures

	fixtures = setupAnalyzeFixtures(t) // TODO: rename to be more generic
	defer fixtures.removeAll(t)

	// TODO: make this better
	// TODO: see about ignoring changes to *analyzed.toml
	analyzedPath := filepath.Join("testdata", "exporter", "container", "layers", "analyzed.toml")
	analyzedMD := assertAnalyzedMetadata(t, analyzedPath)
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: fixtures.regRunImage} // TODO: check if metadata on fixture matches metadata in analyzed.toml
	lifecycle.WriteTOML(analyzedPath, analyzedMD)

	analyzedPath = filepath.Join("testdata", "exporter", "container", "layers", "daemon-analyzed.toml")
	analyzedMD = assertAnalyzedMetadata(t, analyzedPath)
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: fixtures.daemonRunImage} // TODO: check if metadata on fixture matches metadata in analyzed.toml
	lifecycle.WriteTOML(analyzedPath, analyzedMD)

	analyzedPath = filepath.Join("testdata", "exporter", "container", "layers", "some-analyzed.toml")
	analyzedMD = assertAnalyzedMetadata(t, analyzedPath)
	analyzedMD.Image = &platform.ImageIdentifier{Reference: fixtures.someAppImage}   // TODO: check if metadata on fixture matches metadata in analyzed.toml
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: fixtures.regRunImage} // TODO: check if metadata on fixture matches metadata in analyzed.toml
	lifecycle.WriteTOML(analyzedPath, analyzedMD)
	// end TODO

	h.MakeAndCopyLifecycle(t, daemonOS, daemonArch, exporterBinaryDir)
	h.DockerBuild(t, exportImage, exportDockerContext)
	defer h.DockerImageRemove(t, exportImage)

	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "acceptance-exporter/"+platformAPI.String(), testExporterFunc(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testExporterFunc(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		when("daemon case", func() {
			when("first build", func() {
				when("app", func() {
					it("is created", func() {
						exportFlags := []string{"-daemon"}
						if api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0 {
							exportFlags = append(exportFlags, []string{"-run-image", fixtures.regRunImage}...)
						} else {
							exportFlags = append(exportFlags, []string{"-analyzed", "/layers/daemon-analyzed.toml"}...) // TODO: understand why this fixes platform 0.7 but other platforms are fine
						}

						exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
						exportedImageName := "some-exported-image-" + h.RandString(10)
						exportArgs = append(exportArgs, exportedImageName)

						output := h.DockerRun(t,
							exportImage,
							h.WithFlags(append(
								dockerSocketMount,
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_REGISTRY_AUTH="+fixtures.regAuthConfig,
								"--network", registryNetwork,
							)...),
							h.WithArgs(exportArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+exportedImageName)

						inspect, _, err := h.DockerCli(t).ImageInspectWithRaw(context.TODO(), exportedImageName) // TODO: make test helper
						h.AssertNil(t, err)
						h.AssertEq(t, inspect.Os, daemonOS)
						h.AssertEq(t, inspect.Architecture, daemonArch)
					})
				})
				//when("cache", func() {
				//	when("cache directory case", func() {
				//		it("is updated", func() {
				//
				//		})
				//	})
				//})
			})
			//when("next build", func() {
			//	when("app", func() {
			//		it("is updated", func() {
			//
			//		})
			//	})
			//	when("cache", func() {
			//		when("cache directory case", func() {
			//			it("is updated", func() {
			//
			//			})
			//		})
			//	})
			//})
		})

		when("registry case", func() {
			when("first build", func() {
				when("app", func() {
					it("is created", func() {
						var exportFlags []string
						if api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0 {
							exportFlags = append(exportFlags, []string{"-run-image", fixtures.regRunImage}...)
						}

						exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
						exportedImageName := testRegistry.RepoName("some-exported-image-" + h.RandString(10))
						exportArgs = append(exportArgs, exportedImageName)

						output := h.DockerRun(t,
							exportImage,
							h.WithFlags(
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_REGISTRY_AUTH="+fixtures.regAuthConfig,
								"--network", registryNetwork,
							),
							h.WithArgs(exportArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+exportedImageName)

						h.Run(t, exec.Command("docker", "pull", exportedImageName))                              // TODO: cleanup this image
						inspect, _, err := h.DockerCli(t).ImageInspectWithRaw(context.TODO(), exportedImageName) // TODO: make test helper
						h.AssertNil(t, err)
						h.AssertEq(t, inspect.Os, daemonOS)
						h.AssertEq(t, inspect.Architecture, daemonArch)
					})
				})
				when("cache", func() {
					when("cache image case", func() {
						it("is created", func() {
							cacheImageName := testRegistry.RepoName("some-cache-image-" + h.RandString(10))
							exportFlags := []string{"-cache-image", cacheImageName}
							if api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0 {
								exportFlags = append(exportFlags, "-run-image", fixtures.regRunImage)
							}

							exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
							exportedImageName := testRegistry.RepoName("some-exported-image-" + h.RandString(10))
							exportArgs = append(exportArgs, exportedImageName)

							output := h.DockerRun(t,
								exportImage,
								h.WithFlags(
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--env", "CNB_REGISTRY_AUTH="+fixtures.regAuthConfig,
									"--network", registryNetwork,
								),
								h.WithArgs(exportArgs...),
							)
							h.AssertStringContains(t, output, "Saving "+exportedImageName)

							h.Run(t, exec.Command("docker", "pull", exportedImageName))                              // TODO: cleanup this image
							inspect, _, err := h.DockerCli(t).ImageInspectWithRaw(context.TODO(), exportedImageName) // TODO: make test helper
							h.AssertNil(t, err)
							h.AssertEq(t, inspect.Os, daemonOS)
							h.AssertEq(t, inspect.Architecture, daemonArch)

							h.Run(t, exec.Command("docker", "pull", cacheImageName))                                // TODO: cleanup this image
							inspect, _, err = h.DockerCli(t).ImageInspectWithRaw(context.TODO(), cacheImageName) // TODO: make test helper
							h.AssertNil(t, err)
							h.AssertEq(t, inspect.Os, daemonOS)
							h.AssertEq(t, inspect.Architecture, daemonArch)
						})
					})
				})
			})
			//when("next build", func() {
			//	when("app", func() {
			//		it("is updated", func() {
			//
			//		})
			//	})
			//	when("cache", func() {
			//		when("cache image case", func() {
			//			it("is updated", func() {
			//
			//			})
			//		})
			//	})
			//})
		})
	}
}
