//go:build acceptance
// +build acceptance

package acceptance

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/internal/selective"
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

		when("kaniko case", func() {
			var kanikoDir, buildImageDigest string

			it.Before(func() {
				var err error
				kanikoDir, err = os.MkdirTemp("", "lifecycle-acceptance")
				h.AssertNil(t, err)

				// push "builder" image to test registry
				h.Run(t, exec.Command("docker", "tag", extendImage, extendTest.RegRepoName(extendImage)))
				h.AssertNil(t, h.PushImage(h.DockerCli(t), extendTest.RegRepoName(extendImage), extendTest.targetRegistry.registry.EncodedLabeledAuth()))

				// warm kaniko cache - this mimics what the analyzer or restorer would have done
				os.Setenv("DOCKER_CONFIG", extendTest.targetRegistry.dockerConfigDir)
				ref, auth, err := auth.ReferenceForRepoName(authn.DefaultKeychain, extendTest.RegRepoName(extendImage))
				h.AssertNil(t, err)
				remoteImage, err := remote.Image(ref, remote.WithAuth(auth))
				h.AssertNil(t, err)
				buildImageHash, err := remoteImage.Digest()
				h.AssertNil(t, err)
				buildImageDigest = buildImageHash.String()
				baseCacheDir := filepath.Join(kanikoDir, "cache", "base")
				h.AssertNil(t, os.MkdirAll(baseCacheDir, 0755))
				layoutPath, err := selective.Write(filepath.Join(baseCacheDir, buildImageDigest), empty.Index)
				h.AssertNil(t, err)
				h.AssertNil(t, layoutPath.AppendImage(remoteImage))
			})

			it.After(func() {
				_ = os.RemoveAll(kanikoDir)
			})

			when("extending the build image", func() {
				it("succeeds", func() {
					extendArgs := []string{
						ctrPath(extenderPath),
						"-generated", "/layers/generated",
						"-log-level", "debug",
						"-gid", "1000",
						"-uid", "1234",
						"oci:/kaniko/cache/base/" + buildImageDigest,
					}

					t.Log("first build extends the build image by running Dockerfile commands")
					firstOutput := h.DockerRunWithCombinedOutput(t,
						extendImage,
						h.WithFlags(
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--volume", fmt.Sprintf("%s:/kaniko", kanikoDir),
						),
						h.WithArgs(extendArgs...),
					)
					h.AssertStringDoesNotContain(t, firstOutput, "Did not find cache key, pulling remote image...")
					h.AssertStringContains(t, firstOutput, "ca-certificates")
					h.AssertStringContains(t, firstOutput, "Hello Extensions buildpack\ncurl") // output by buildpack, shows that curl was installed on the build image
					t.Log("sets environment variables from the extended build image in the build context")
					h.AssertStringContains(t, firstOutput, "CNB_STACK_ID for buildpack: stack-id-from-ext-tree")

					t.Log("cleans the kaniko directory")
					fis, err := ioutil.ReadDir(kanikoDir)
					h.AssertNil(t, err)
					h.AssertEq(t, len(fis), 1) // 1: /kaniko/cache

					t.Log("second build extends the build image by pulling from the cache directory")
					secondOutput := h.DockerRunWithCombinedOutput(t,
						extendImage,
						h.WithFlags(
							"--env", "CNB_PLATFORM_API="+platformAPI,
							"--volume", fmt.Sprintf("%s:/kaniko", kanikoDir),
						),
						h.WithArgs(extendArgs...),
					)
					h.AssertStringDoesNotContain(t, secondOutput, "Did not find cache key, pulling remote image...")
					h.AssertStringDoesNotContain(t, secondOutput, "ca-certificates")
					h.AssertStringContains(t, secondOutput, "Hello Extensions buildpack\ncurl") // output by buildpack, shows that curl is still installed in the unpacked cached layer
				})
			})
		})
	}
}
