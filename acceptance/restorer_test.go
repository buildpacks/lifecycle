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

	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	restoreDockerContext = filepath.Join("testdata", "restorer")
	restorerBinaryDir    = filepath.Join("testdata", "restorer", "container", "cnb", "lifecycle")
	restorerImage        = "lifecycle/acceptance/restorer"
)

func TestRestorer(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "Restorer acceptance tests are not yet supported on Windows")

	rand.Seed(time.Now().UTC().UnixNano())

	h.MakeAndCopyLifecycle(t, "linux", restorerBinaryDir)
	h.DockerBuild(t, restorerImage, restoreDockerContext)
	defer h.DockerImageRemove(t, restorerImage)

	spec.Run(t, "acceptance-restorer", testRestorer, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testRestorer(t *testing.T, when spec.G, it spec.S) {
	when("called with arguments", func() {
		it("errors", func() {
			command := exec.Command("docker", "run", "--rm", restorerImage, "some-arg")
			output, err := command.CombinedOutput()
			h.AssertNotNil(t, err)
			expected := "failed to parse arguments: received unexpected Args"
			h.AssertStringContains(t, string(output), expected)
		})
	})

	when("called without any cache flag", func() {
		it("outputs it will not restore cache layer data", func() {
			command := exec.Command("docker", "run", "--rm", restorerImage)
			output, err := command.CombinedOutput()
			h.AssertNil(t, err)
			expected := "Not restoring cached layer data, no cache flag specified"
			h.AssertStringContains(t, string(output), expected)
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
					h.WithArgs("-cache-dir", "/cache"),
				)

				// check no content is not present from unused buildpack
				unusedBpLayer := filepath.Join(copyDir, "layers", "unused_buildpack")
				h.AssertPathDoesNotExist(t, unusedBpLayer)
			})
		})
	})
}
