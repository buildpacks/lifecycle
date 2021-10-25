//go:build acceptance
// +build acceptance

package acceptance

import (
	"math/rand"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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

		it.After(func() {
			h.DockerImageRemove(t, createdImageName)
		})

		when("daemon case", func() {
			when("first build", func() {
				when("app", func() {
					it("is created", func() {
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
			})
		})

		when("registry case", func() {
			when("first build", func() {
				when("app", func() {
					it("is created", func() {
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
			})
		})
	}
}
