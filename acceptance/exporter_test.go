package acceptance

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	ih "github.com/buildpacks/imgutil/testhelpers"

	"github.com/buildpacks/lifecycle/acceptance/variables"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	exporterBinaryDir        = filepath.Join("testdata", "exporter", "export-image", "container", "cnb", "lifecycle")
	exporterDockerContext    = filepath.Join("testdata", "exporter", "export-image")
	exporterRunDockerContext = filepath.Join("testdata", "exporter", "run-image")
	exporterImage            = "lifecycle/acceptance/exporter"
	exporterRunImage         = "lifecycle/acceptance/exporter-run"
	exporterPath             = "/cnb/lifecycle/exporter"
)

func TestExporter(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "These tests need to be adapted to work on Windows")
	rand.Seed(time.Now().UTC().UnixNano())

	info, err := h.DockerCli(t).Info(context.TODO())
	h.AssertNil(t, err)
	daemonOS = info.OSType

	// Setup registry

	dockerConfigDir, err := ioutil.TempDir("", "test.docker.config.dir")
	h.AssertNil(t, err)
	defer os.RemoveAll(dockerConfigDir)

	registry = ih.NewDockerRegistryWithAuth(dockerConfigDir)
	registry.Start(t)
	defer registry.Stop(t)

	os.Setenv("DOCKER_CONFIG", registry.DockerDirectory)

	// Setup test container

	h.MakeAndCopyLifecycle(t, daemonOS, exporterBinaryDir)
	h.DockerBuild(t,
		exporterImage,
		exporterDockerContext,
		h.WithFlags("-f", filepath.Join(exporterDockerContext, variables.DockerfileName)),
	)
	defer h.DockerImageRemove(t, exporterImage)

	h.DockerBuild(t,
		exporterRunImage,
		exporterRunDockerContext,
		h.WithFlags("-f", filepath.Join(exporterRunDockerContext, variables.DockerfileName)),
	)
	defer h.DockerImageRemove(t, exporterRunImage)

	spec.Run(t, "acceptance-exporter", testExporter, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testExporter(t *testing.T, when spec.G, it spec.S) {
	var newImageName string

	when("called", func() {
		it.Before(func() {
			newImageName = "test-container-" + h.RandString(10)
		})

		it.After(func() {
			h.DockerImageRemove(t, newImageName)
		})

		it("creates an image", func() {
			h.SkipIf(t, runtime.GOOS == "windows", "Not relevant on Windows")

			output := h.DockerRun(t,
				exporterImage,
				h.WithFlags("--env", "CNB_REGISTRY_AUTH={}"),
				h.WithBash(fmt.Sprintf("%s -run-image %s %s", exporterPath, exporterRunImage, newImageName)),
			)

			h.DockerImageExists(t, newImageName)
			h.AssertMatch(t, output, "foobar")
			// TODO verify that newImage has the right layers
		})
	})
}
