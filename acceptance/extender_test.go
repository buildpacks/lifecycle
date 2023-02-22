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
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/auth"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/internal/selective"
	"github.com/buildpacks/lifecycle/platform"
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
			var kanikoDir, analyzedPath string

			it.Before(func() {
				var err error
				kanikoDir, err = os.MkdirTemp("", "lifecycle-acceptance")
				h.AssertNil(t, err)

				// push base image to test registry
				h.Run(t, exec.Command("docker", "tag", extendImage, extendTest.RegRepoName(extendImage)))
				h.AssertNil(t, h.PushImage(h.DockerCli(t), extendTest.RegRepoName(extendImage), extendTest.targetRegistry.registry.EncodedLabeledAuth()))

				// mimic what the restorer would have done in the previous phase:

				// warm kaniko cache

				// get remote image
				os.Setenv("DOCKER_CONFIG", extendTest.targetRegistry.dockerConfigDir)
				ref, auth, err := auth.ReferenceForRepoName(authn.DefaultKeychain, extendTest.RegRepoName(extendImage))
				h.AssertNil(t, err)
				remoteImage, err := remote.Image(ref, remote.WithAuth(auth))
				h.AssertNil(t, err)
				baseImageHash, err := remoteImage.Digest()
				h.AssertNil(t, err)
				baseImageDigest := baseImageHash.String()
				baseCacheDir := filepath.Join(kanikoDir, "cache", "base")
				h.AssertNil(t, os.MkdirAll(baseCacheDir, 0755))
				// write image at layout path
				selectivePath := filepath.Join(baseCacheDir, baseImageDigest)
				layoutPath, err := selective.Write(selectivePath, empty.Index)
				h.AssertNil(t, err)
				h.AssertNil(t, layoutPath.AppendImage(remoteImage))
				t.Logf("Saved selective image %s", selectivePath)

				// write image reference in analyzed.toml
				analyzedMD := platform.AnalyzedMetadata{
					BuildImage: &platform.ImageIdentifier{
						Reference: fmt.Sprintf("%s@%s", extendTest.RegRepoName(extendImage), baseImageDigest),
					},
					RunImage: &platform.RunImage{
						Reference: fmt.Sprintf("%s@%s", extendTest.RegRepoName(extendImage), baseImageDigest),
						Extend:    true,
					},
				}
				analyzedPath = h.TempFile(t, "", "analyzed.toml")
				h.AssertNil(t, encoding.WriteTOML(analyzedPath, analyzedMD))
			})

			it.After(func() {
				_ = os.RemoveAll(kanikoDir)
			})

			when("extending the build image", func() {
				it("succeeds", func() {
					extendArgs := []string{
						ctrPath(extenderPath),
						"-analyzed", "/layers/analyzed.toml",
						"-generated", "/layers/generated",
						"-log-level", "debug",
						"-gid", "1000",
						"-uid", "1234",
					}

					extendFlags := []string{
						"--env", "CNB_PLATFORM_API=" + platformAPI,
						"--volume", fmt.Sprintf("%s:/layers/analyzed.toml", analyzedPath),
						"--volume", fmt.Sprintf("%s:/kaniko", kanikoDir),
					}

					t.Log("first build extends the build image by running Dockerfile commands")
					firstOutput := h.DockerRunWithCombinedOutput(t,
						extendImage,
						h.WithFlags(extendFlags...),
						h.WithArgs(extendArgs...),
					)
					h.AssertStringDoesNotContain(t, firstOutput, "Did not find cache key, pulling remote image...")
					h.AssertStringDoesNotContain(t, firstOutput, "Error while retrieving image from cache: oci")
					h.AssertStringContains(t, firstOutput, "ca-certificates")
					h.AssertStringContains(t, firstOutput, "Hello Extensions buildpack\ncurl") // output by buildpack, shows that curl was installed on the build image
					t.Log("sets environment variables from the extended build image in the build context")
					h.AssertStringContains(t, firstOutput, "CNB_STACK_ID for buildpack: stack-id-from-ext-tree")
					h.AssertStringContains(t, firstOutput, "HOME for buildpack: /home/cnb")

					t.Log("cleans the kaniko directory")
					fis, err := os.ReadDir(kanikoDir)
					h.AssertNil(t, err)
					h.AssertEq(t, len(fis), 1) // 1: /kaniko/cache

					t.Log("second build extends the build image by pulling from the cache directory")
					secondOutput := h.DockerRunWithCombinedOutput(t,
						extendImage,
						h.WithFlags(extendFlags...),
						h.WithArgs(extendArgs...),
					)
					h.AssertStringDoesNotContain(t, secondOutput, "Did not find cache key, pulling remote image...")
					h.AssertStringDoesNotContain(t, secondOutput, "Error while retrieving image from cache: oci")
					h.AssertStringDoesNotContain(t, secondOutput, "ca-certificates")            // shows that cache layer was used
					h.AssertStringContains(t, secondOutput, "Hello Extensions buildpack\ncurl") // output by buildpack, shows that curl is still installed in the unpacked cached layer
				})
			})

			when("extending the run image", func() {
				it.Before(func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "Platform API < 0.12 does not support run image extension")
				})

				it("succeeds", func() {
					extendArgs := []string{
						ctrPath(extenderPath),
						"-analyzed", "/layers/analyzed.toml",
						"-extended", "/layers/extended",
						"-generated", "/layers/generated",
						"-kind", "run",
						"-log-level", "debug",
						"-gid", "1000",
						"-uid", "1234",
					}

					extendFlags := []string{
						"--env", "CNB_PLATFORM_API=" + platformAPI,
						"--volume", fmt.Sprintf("%s:/layers/analyzed.toml", analyzedPath),
						"--volume", fmt.Sprintf("%s:/kaniko", kanikoDir),
					}

					t.Log("first build extends the build image by running Dockerfile commands")
					firstOutput := h.DockerRunWithCombinedOutput(t,
						extendImage,
						h.WithFlags(extendFlags...),
						h.WithArgs(extendArgs...),
					)
					h.AssertStringDoesNotContain(t, firstOutput, "Did not find cache key, pulling remote image...")
					h.AssertStringDoesNotContain(t, firstOutput, "Error while retrieving image from cache: oci")
					h.AssertStringContains(t, firstOutput, "ca-certificates")
					h.AssertStringContains(t, firstOutput, "Hello Extensions buildpack\ncurl") // output by buildpack, shows that curl was installed on the build image
					t.Log("sets environment variables from the extended build image in the build context")
					h.AssertStringContains(t, firstOutput, "CNB_STACK_ID for buildpack: stack-id-from-ext-tree")
					h.AssertStringContains(t, firstOutput, "HOME for buildpack: /home/cnb")

					t.Log("cleans the kaniko directory")
					fis, err := os.ReadDir(kanikoDir)
					h.AssertNil(t, err)
					h.AssertEq(t, len(fis), 1) // 1: /kaniko/cache

					t.Log("second build extends the build image by pulling from the cache directory")
					secondOutput := h.DockerRunWithCombinedOutput(t,
						extendImage,
						h.WithFlags(extendFlags...),
						h.WithArgs(extendArgs...),
					)
					h.AssertStringDoesNotContain(t, secondOutput, "Did not find cache key, pulling remote image...")
					h.AssertStringDoesNotContain(t, secondOutput, "Error while retrieving image from cache: oci")
					h.AssertStringDoesNotContain(t, secondOutput, "ca-certificates")            // shows that cache layer was used
					h.AssertStringContains(t, secondOutput, "Hello Extensions buildpack\ncurl") // output by buildpack, shows that curl is still installed in the unpacked cached layer
				})
			})
		})
	}
}
