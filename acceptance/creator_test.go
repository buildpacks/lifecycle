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

	"github.com/buildpacks/lifecycle/api"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	creatorBinaryDir    = filepath.Join("testdata", "creator", "container", "cnb", "lifecycle")
	createDockerContext = filepath.Join("testdata", "creator")
	createImage         = "lifecycle/acceptance/creator"
	creatorPath         = "/cnb/lifecycle/creator"
)

func TestCreator(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "Creator acceptance tests are not yet supported on Windows")

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
	targetDockerConfig := filepath.Join("testdata", "creator", "container", "docker-config")
	h.AssertNil(t, os.RemoveAll(filepath.Join(targetDockerConfig, "config.json")))
	h.RecursiveCopy(t, testRegistry.DockerDirectory, targetDockerConfig)

	// end TODO

	// Setup fixtures

	fixtures = setupAnalyzeFixtures(t) // TODO: rename to be more generic
	defer fixtures.removeAll(t)

	h.MakeAndCopyLifecycle(t, daemonOS, daemonArch, creatorBinaryDir)
	h.DockerBuild(t, createImage, createDockerContext)
	defer h.DockerImageRemove(t, createImage)

	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "acceptance-creator/"+platformAPI.String(), testCreatorFunc(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testCreatorFunc(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		when("daemon case", func() {
			when("first build", func() {
				when("app", func() {
					it("is created", func() {
						createFlags := []string{"-daemon"}
						createFlags = append(createFlags, []string{"-run-image", fixtures.regRunImage}...)

						createArgs := append([]string{ctrPath(creatorPath)}, createFlags...)
						createdImageName := "some-created-image-" + h.RandString(10)
						createArgs = append(createArgs, createdImageName)

						output := h.DockerRun(t,
							createImage,
							h.WithFlags(append(
								dockerSocketMount,
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_REGISTRY_AUTH="+fixtures.regAuthConfig,
								"--network", registryNetwork,
							)...),
							h.WithArgs(createArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+createdImageName)

						inspect, _, err := h.DockerCli(t).ImageInspectWithRaw(context.TODO(), createdImageName) // TODO: make test helper
						h.AssertNil(t, err)
						h.AssertEq(t, inspect.Os, daemonOS)
						h.AssertEq(t, inspect.Architecture, daemonArch)

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
						var createFlags []string
						createFlags = append(createFlags, []string{"-run-image", fixtures.regRunImage}...)

						createArgs := append([]string{ctrPath(creatorPath)}, createFlags...)
						createdImageName := testRegistry.RepoName("some-created-image-" + h.RandString(10))
						createArgs = append(createArgs, createdImageName)

						output := h.DockerRun(t,
							createImage,
							h.WithFlags(
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_REGISTRY_AUTH="+fixtures.regAuthConfig,
								"--network", registryNetwork,
							),
							h.WithArgs(createArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+createdImageName)

						h.Run(t, exec.Command("docker", "pull", createdImageName))                              // TODO: cleanup this image
						inspect, _, err := h.DockerCli(t).ImageInspectWithRaw(context.TODO(), createdImageName) // TODO: make test helper
						h.AssertNil(t, err)
						h.AssertEq(t, inspect.Os, daemonOS)
						h.AssertEq(t, inspect.Architecture, daemonArch)

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
				//when("cache", func() {
				//	when("cache image case", func() {
				//		it("is created", func() {
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
