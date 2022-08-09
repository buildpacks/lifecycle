//go:build acceptance
// +build acceptance

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
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	extendImage          string
	extendRegAuthConfig  string
	extendRegNetwork     string
	extenderPath         string
	extendDaemonFixtures *daemonImageFixtures
	extendRegFixtures    *regImageFixtures
	extendTest           *PhaseTest
)

func TestExtender(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "Extender is not supported on Windows")

	rand.Seed(time.Now().UTC().UnixNano())

	testImageDockerContext := filepath.Join("testdata", "extender")
	extendTest = NewPhaseTest(t, "extender", testImageDockerContext)
	extendTest.Start(t)
	defer extendTest.Stop(t)

	extendImage = extendTest.testImageRef
	extenderPath = extendTest.containerBinaryPath
	cacheFixtureDir = filepath.Join("testdata", "extender", "cache-dir")
	extendRegAuthConfig = extendTest.targetRegistry.authConfig
	extendRegNetwork = extendTest.targetRegistry.network
	extendDaemonFixtures = extendTest.targetDaemon.fixtures
	extendRegFixtures = extendTest.targetRegistry.fixtures

	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "acceptance-extender/"+platformAPI.String(), testExtenderFunc(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testExtenderFunc(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		it.Before(func() {
			h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.10"), "")
		})

		when.Pend("daemon case", func() {})

		when("kaniko case", func() {
			var workDir, buildImageDigest string

			it.Before(func() {
				var err error
				workDir, err = filepath.Abs(filepath.Join("testdata", "extender", "mounts", "workspace"))
				h.AssertNil(t, err)
				// clean cache directory
				h.AssertNil(t, os.RemoveAll(workDir)) // TODO: make tempdir
				h.AssertNil(t, os.MkdirAll(workDir, 0755))

				// push "builder" image to test registry
				h.Run(t, exec.Command("docker", "tag", extendImage, extendTest.RegRepoName(extendImage)))
				h.AssertNil(t, h.PushImage(h.DockerCli(t), extendTest.RegRepoName(extendImage), extendTest.targetRegistry.registry.EncodedLabeledAuth()))

				// warm kaniko cache
				h.DockerRun(t,
					"gcr.io/kaniko-project/warmer:latest",
					h.WithFlags(
						"--env", "DOCKER_CONFIG=/docker-config",
						"--volume", fmt.Sprintf("%s:/docker-config", extendTest.targetRegistry.dockerConfigDir),
						"--volume", fmt.Sprintf("%s:/workspace", workDir),
					),
					h.WithArgs(
						"--cache-dir=/workspace/cache",
						fmt.Sprintf("--image=%s", extendTest.RegRepoName(extendImage)),
					),
				)

				// get digest
				// TODO: this is a hacky way to get the digest and should be improved
				indexFile, err := filepath.Glob(filepath.Join(workDir, "cache", "sha256:*.json"))
				h.AssertNil(t, err)
				h.AssertEq(t, len(indexFile), 1)
				buildImageDigest = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(indexFile[0]), "sha256:"), ".json")
			})

			when("extending the build image", func() {
				it("succeeds", func() {
					extendArgs := []string{
						ctrPath(extenderPath),
						//"-cache-image", "oci:/kaniko/cache-dir/cache-image", // TODO: make configurable
						"-generated", "/layers/generated",
						"-log-level", "debug",
						"-gid", "1000",
						"-uid", "1234",
						extendTest.RegRepoName(extendImage) + "@sha256:" + buildImageDigest,
					}
					kanikoDir, err := filepath.Abs(filepath.Join("testdata", "extender", "mounts", "kaniko"))
					h.AssertNil(t, err)
					layersDir, err := filepath.Abs(filepath.Join("testdata", "extender", "mounts", "layers"))
					h.AssertNil(t, err)

					t.Log("extends the build image")
					firstOutput := h.DockerRun(t,
						extendImage,
						h.WithFlags(
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--volume", fmt.Sprintf("%s:/kaniko", kanikoDir),
							"--volume", fmt.Sprintf("%s:/layers", layersDir),
							"--volume", fmt.Sprintf("%s:/workspace", workDir),
						),
						h.WithArgs(extendArgs...),
					)
					h.AssertStringContains(t, firstOutput, "ca-certificates")
					h.AssertStringContains(t, firstOutput, "Hello Extensions buildpack\ncurl 7.58.0") // output by buildpack

					t.Log("uses the cache directory")
					secondOutput := h.DockerRun(t,
						extendImage,
						h.WithFlags(
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--volume", fmt.Sprintf("%s:/kaniko", kanikoDir), // TODO: keep this from growing indefinitely
							"--volume", fmt.Sprintf("%s:/layers", layersDir),
							"--volume", fmt.Sprintf("%s:/workspace", workDir), // contains the cache dir
						),
						h.WithArgs(extendArgs...),
					)
					h.AssertStringDoesNotContain(t, secondOutput, "ca-certificates")
				})
			})
		})
	}
}
