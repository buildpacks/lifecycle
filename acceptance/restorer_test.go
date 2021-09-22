// +build acceptance

package acceptance

import (
	"io/ioutil"
	"math/rand"
	"os"
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
	restoreDockerContext = filepath.Join("testdata", "restorer")
	restorerBinaryDir    = filepath.Join("testdata", "restorer", "container", "cnb", "lifecycle")
	restorerImage        = "lifecycle/acceptance/restorer"
)

func TestRestorer(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "Restorer acceptance tests are not yet supported on Windows")
	h.SkipIf(t, runtime.GOARCH != "amd64", "Restorer acceptance tests are not yet supported on non-amd64")

	rand.Seed(time.Now().UTC().UnixNano())

	h.MakeAndCopyLifecycle(t, "linux", "amd64", restorerBinaryDir)
	h.DockerBuild(t, restorerImage, restoreDockerContext)
	defer h.DockerImageRemove(t, restorerImage)

	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "acceptance-restorer/"+platformAPI.String(), testRestorerFunc(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testRestorerFunc(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		when("called with arguments", func() {
			it("errors", func() {
				command := exec.Command("docker", "run", "--rm", restorerImage, "some-arg")
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "failed to parse arguments: received unexpected Args"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("called with -analyzed", func() {
			it("errors", func() {
				h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 supports -analyzed flag")
				command := exec.Command("docker", "run", "--rm", restorerImage, "-analyzed some-file-location")
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "flag provided but not defined: -analyzed"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("called with -skip-layers", func() {
			it("errors", func() {
				h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "Platform API >= 0.7 supports -skip-layers flag")
				command := exec.Command("docker", "run", "--rm", restorerImage, "-skip-layers true")
				output, err := command.CombinedOutput()
				h.AssertNotNil(t, err)
				expected := "flag provided but not defined: -skip-layers"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("called without any cache flag", func() {
			it("outputs it will not restore cache layer data", func() {
				command := exec.Command("docker", "run", "--rm", "--env", "CNB_PLATFORM_API="+platformAPI, restorerImage)
				output, err := command.CombinedOutput()
				h.AssertNil(t, err)
				expected := "Not restoring cached layer data, no cache flag specified"
				h.AssertStringContains(t, string(output), expected)
			})
		})

		when("analyzed.toml exists with app metadata", func() {
			var copyDir, containerName string
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
				os.RemoveAll(copyDir)
			})

			it("restores app metadata", func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.7"), "Platform API < 0.7 does not restore app metadata")
				output := h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					ctrPath("/layers"),
					restorerImage,
					h.WithFlags(append(
						dockerSocketMount,
						"--env", "CNB_PLATFORM_API="+platformAPI,
					)...),
					h.WithArgs(),
				)

				h.AssertStringContains(t, output, "Restoring metadata for \"some-buildpack-id:launch-layer\"")
			})

		})

		when("using cache-dir", func() {
			when("there is cache present from a previous build", func() {
				var copyDir, containerName string

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
					os.RemoveAll(copyDir)
				})

				it("restores cached layer data", func() {
					h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						"/layers",
						restorerImage,
						h.WithFlags("--env", "CNB_PLATFORM_API="+platformAPI),
						h.WithArgs("-cache-dir", "/cache"),
					)

					// check restored cache file is present
					cachedFile := filepath.Join(copyDir, "layers", "cacher_buildpack", "cached-layer", "data")
					h.AssertPathExists(t, cachedFile)

					// check restored cache file content is correct
					contents, err := ioutil.ReadFile(cachedFile)
					h.AssertNil(t, err)
					h.AssertEq(t, string(contents), "cached-data\n")
				})

				it("does not restore cache=true layers not in cache", func() {
					output := h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						"/layers",
						restorerImage,
						h.WithFlags("--env", "CNB_PLATFORM_API="+platformAPI),
						h.WithArgs("-cache-dir", "/cache"),
					)

					// check uncached layer is not restored
					uncachedFile := filepath.Join(copyDir, "layers", "cacher_buildpack", "uncached-layer")
					h.AssertPathDoesNotExist(t, uncachedFile)

					// check output to confirm why this layer was not restored from cache
					h.AssertStringContains(t, string(output), "Removing \"cacher_buildpack:layer-not-in-cache\", not in cache")
				})

				it("does not restore unused buildpack layer data", func() {
					h.DockerRunAndCopy(t,
						containerName,
						copyDir,
						"/layers",
						restorerImage,
						h.WithFlags("--env", "CNB_PLATFORM_API="+platformAPI),
						h.WithArgs("-cache-dir", "/cache"),
					)

					// check no content is not present from unused buildpack
					unusedBpLayer := filepath.Join(copyDir, "layers", "unused_buildpack")
					h.AssertPathDoesNotExist(t, unusedBpLayer)
				})
			})
		})
	}
}
