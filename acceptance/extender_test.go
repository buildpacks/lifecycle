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
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
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

const (
	// Log message emitted by kaniko;
	// if we provide cache directory as an option, kaniko looks there for the base image as a tarball;
	// however the base image is in OCI layout format, so we fail to initialize the base image;
	// we manage to provide the base image because we override image.RetrieveRemoteImage,
	// but the log message could be confusing to end users, hence we check that it is not printed.
	msgErrRetrievingImageFromCache = "Error while retrieving image from cache"
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
		if platformAPI.LessThan("0.10") {
			continue
		}
		spec.Run(t, "acceptance-extender/"+platformAPI.String(), testExtenderFunc(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testExtenderFunc(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		it.Before(func() {
			h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.10"), "")
		})

		when("kaniko case", func() {
			var extendedDir, kanikoDir, analyzedPath string

			it.Before(func() {
				var err error
				extendedDir, err = os.MkdirTemp("", "lifecycle-acceptance")
				h.AssertNil(t, err)
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

				layoutPath, err := selective.Write(filepath.Join(baseCacheDir, baseImageDigest), empty.Index)
				h.AssertNil(t, err)
				h.AssertNil(t, layoutPath.AppendImage(remoteImage))

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
				_ = os.RemoveAll(extendedDir)
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
					h.AssertStringDoesNotContain(t, firstOutput, msgErrRetrievingImageFromCache)
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
					h.AssertStringDoesNotContain(t, secondOutput, msgErrRetrievingImageFromCache)
					h.AssertStringDoesNotContain(t, secondOutput, "ca-certificates")                                                             // shows that first cache layer was used
					h.AssertStringDoesNotContain(t, secondOutput, "No cached layer found for cmd RUN apt-get update && apt-get install -y tree") // shows that second cache layer was used
					h.AssertStringContains(t, secondOutput, "Hello Extensions buildpack\ncurl")                                                  // output by buildpack, shows that curl is still installed in the unpacked cached layer
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
						"--volume", fmt.Sprintf("%s:/layers/extended", extendedDir),
						"--volume", fmt.Sprintf("%s:/kaniko", kanikoDir),
					}

					t.Log("first build extends the run image by running Dockerfile commands")
					firstOutput := h.DockerRunWithCombinedOutput(t,
						extendImage,
						h.WithFlags(extendFlags...),
						h.WithArgs(extendArgs...),
					)
					h.AssertStringDoesNotContain(t, firstOutput, msgErrRetrievingImageFromCache)
					h.AssertStringContains(t, firstOutput, "ca-certificates")
					h.AssertStringContains(t, firstOutput, "No cached layer found for cmd RUN apt-get update && apt-get install -y tree")
					t.Log("does not run the build phase")
					h.AssertStringDoesNotContain(t, firstOutput, "Hello Extensions buildpack\ncurl")
					t.Log("outputs extended image layers to the extended directory")
					images, err := os.ReadDir(filepath.Join(extendedDir, "run"))
					h.AssertNil(t, err)
					h.AssertEq(t, len(images), 1) // sha256:<extended image digest>
					assertExpectedImage(t, filepath.Join(extendedDir, "run", images[0].Name()))
					t.Log("cleans the kaniko directory")
					caches, err := os.ReadDir(kanikoDir)
					h.AssertNil(t, err)
					h.AssertEq(t, len(caches), 1) // 1: /kaniko/cache

					t.Log("second build extends the build image by pulling from the cache directory")
					secondOutput := h.DockerRunWithCombinedOutput(t,
						extendImage,
						h.WithFlags(extendFlags...),
						h.WithArgs(extendArgs...),
					)
					h.AssertStringDoesNotContain(t, secondOutput, msgErrRetrievingImageFromCache)
					h.AssertStringDoesNotContain(t, secondOutput, "ca-certificates")                                                             // shows that first cache layer was used
					h.AssertStringDoesNotContain(t, secondOutput, "No cached layer found for cmd RUN apt-get update && apt-get install -y tree") // shows that second cache layer was used
					t.Log("does not run the build phase")
					h.AssertStringDoesNotContain(t, secondOutput, "Hello Extensions buildpack\ncurl")
					t.Log("outputs extended image layers to the extended directory")
					images, err = os.ReadDir(filepath.Join(extendedDir, "run"))
					h.AssertNil(t, err)
					h.AssertEq(t, len(images), 1) // sha256:<first extended image digest>
					t.Log("cleans the kaniko directory")
					caches, err = os.ReadDir(kanikoDir)
					h.AssertNil(t, err)
					h.AssertEq(t, len(caches), 1) // 1: /kaniko/cache
				})
			})
		})
	}
}

func assertExpectedImage(t *testing.T, imagePath string) {
	image, err := readOCI(imagePath)
	h.AssertNil(t, err)
	configFile, err := image.ConfigFile()
	h.AssertNil(t, err)
	h.AssertEq(t, configFile.Config.Labels["io.buildpacks.rebasable"], "false")
	layers, err := image.Layers()
	h.AssertNil(t, err)
	h.AssertEq(t, len(layers), 5) // base (3), curl (1), tree (1)
}

func readOCI(fromPath string) (v1.Image, error) {
	layoutPath, err := layout.FromPath(fromPath)
	if err != nil {
		return nil, fmt.Errorf("getting layout from path: %w", err)
	}
	hash, err := v1.NewHash(filepath.Base(fromPath))
	if err != nil {
		return nil, fmt.Errorf("getting hash from reference '%s': %w", fromPath, err)
	}
	v1Image, err := layoutPath.Image(hash)
	if err != nil {
		return nil, fmt.Errorf("getting image from hash '%s': %w", hash.String(), err)
	}
	return v1Image, nil
}
