package acceptance

import (
	"context"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	ih "github.com/buildpacks/imgutil/testhelpers"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExporter(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())

	info, err := h.DockerCli(t).Info(context.TODO())
	h.AssertNil(t, err)
	daemonOS = info.OSType

	// Setup registry

	dockerConfigDir, err := ioutil.TempDir("", "test.docker.config.dir")
	h.AssertNil(t, err)
	defer os.RemoveAll(dockerConfigDir)

	sharedRegHandler := registry.New(registry.Logger(log.New(ioutil.Discard, "", log.Lshortfile)))
	authRegistry = ih.NewDockerRegistry(ih.WithAuth(dockerConfigDir), ih.WithSharedHandler(sharedRegHandler))
	authRegistry.Start(t)
	defer authRegistry.Stop(t)

	noAuthRegistry = ih.NewDockerRegistry(ih.WithSharedHandler(sharedRegHandler))
	noAuthRegistry.Start(t)
	defer noAuthRegistry.Stop(t)

	// if registry is listening on localhost, use host networking to allow containers to reach it
	registryNetwork = "default"
	if authRegistry.Host == "localhost" {
		registryNetwork = "host"
	}

	os.Setenv("DOCKER_CONFIG", authRegistry.DockerDirectory)
	// Copy docker config directory to export-image container
	targetDockerConfig := filepath.Join("testdata", "exporter", "export-image", "container", "docker-config")
	h.AssertNil(t, os.RemoveAll(filepath.Join(targetDockerConfig, "config.json")))
	h.RecursiveCopy(t, authRegistry.DockerDirectory, targetDockerConfig)

	var (
		exporterBinaryDir   = filepath.Join("testdata", "exporter", "export-image", "container", "cnb", "lifecycle")
		exportDockerContext = filepath.Join("testdata", "exporter", "export-image")
		exportImage         = "lifecycle/acceptance/exporter"
	)

	// Setup test container
	h.MakeAndCopyLifecycle(t, daemonOS, exporterBinaryDir)
	h.DockerBuild(t,
		exportImage,
		exportDockerContext,
		h.WithFlags(
			"-f", filepath.Join(exportDockerContext, dockerfileName),
		),
	)
	defer h.DockerImageRemove(t, exportImage)

	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "acceptance-exporter/"+platformAPI.String(),
			testExporterFunc(platformAPI.String(), exportImage),
			spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testExporterFunc(platformAPI, exportImage string) func(t *testing.T, when spec.G, it spec.S) {
	const (
		exporterPath = "/cnb/lifecycle/exporter"
	)

	return func(t *testing.T, when spec.G, it spec.S) {
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

		when("layout case", func() {
			it("exports image", func() {
				output := h.DockerRunAndCopy(t,
					containerName,
					copyDir,
					ctrPath("/layers"),
					exportImage,
					h.WithFlags(append(
						dockerSocketMount,
						"--env", "CNB_PLATFORM_API="+platformAPI,
					)...),
					h.WithArgs(
						ctrPath(exporterPath),
						"-layout"),
				)

				assertNoRestoreOfAppMetadata(t, copyDir, output)
			})
		})
	}
}
