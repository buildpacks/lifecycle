// +build acceptance

package acceptance

import (
	"context"
	"math/rand"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var (
	exportImage          string
	exportRegAuthConfig  string
	exportRegNetwork     string
	exporterPath         string
	exportDaemonFixtures *daemonImageFixtures
	exportRegFixtures    *regImageFixtures
	exportTest           *PhaseTest
)

func TestExporter(t *testing.T) {
	h.SkipIf(t, runtime.GOOS == "windows", "Exporter acceptance tests are not yet supported on Windows")

	rand.Seed(time.Now().UTC().UnixNano())

	testImageDockerContext := filepath.Join("testdata", "exporter")
	exportTest = NewPhaseTest(t, "exporter", testImageDockerContext)
	exportTest.Start(t, modifyAnalyzedTOMLWithRegRepoName)
	defer exportTest.Stop(t)

	exportImage = exportTest.testImageRef
	exporterPath = exportTest.containerBinaryPath
	cacheFixtureDir = filepath.Join("testdata", "exporter", "cache-dir")
	exportRegAuthConfig = exportTest.targetRegistry.authConfig
	exportRegNetwork = exportTest.targetRegistry.network
	exportDaemonFixtures = exportTest.targetDaemon.fixtures
	exportRegFixtures = exportTest.targetRegistry.fixtures

	rand.Seed(time.Now().UTC().UnixNano())

	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "acceptance-exporter/"+platformAPI.String(), testExporterFunc(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testExporterFunc(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		when("daemon case", func() {
			when("first build", func() {
				when("app", func() {
					it("is created", func() {
						exportFlags := []string{"-daemon"}
						if api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0 {
							exportFlags = append(exportFlags, []string{"-run-image", exportRegFixtures.ReadOnlyRunImage}...)
						} else {
							exportFlags = append(exportFlags, []string{"-analyzed", "/layers/daemon-analyzed.toml"}...) // TODO: understand why this fixes platform 0.7 but other platforms are fine
						}

						exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
						exportedImageName := "some-exported-image-" + h.RandString(10)
						exportArgs = append(exportArgs, exportedImageName)

						output := h.DockerRun(t,
							exportImage,
							h.WithFlags(append(
								dockerSocketMount,
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_REGISTRY_AUTH="+exportRegAuthConfig,
								"--network", exportRegNetwork,
							)...),
							h.WithArgs(exportArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+exportedImageName)

						inspect, _, err := h.DockerCli(t).ImageInspectWithRaw(context.TODO(), exportedImageName) // TODO: make test helper
						h.AssertNil(t, err)
						h.AssertEq(t, inspect.Os, exportTest.targetDaemon.os)
						h.AssertEq(t, inspect.Architecture, exportTest.targetDaemon.arch)
					})
				})
			})
		})

		when("registry case", func() {
			when("first build", func() {
				when("app", func() {
					it("is created", func() {
						var exportFlags []string
						if api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0 {
							exportFlags = append(exportFlags, []string{"-run-image", exportRegFixtures.ReadOnlyRunImage}...)
						}

						exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
						exportedImageName := exportTest.targetRegistry.registry.RepoName("some-exported-image-" + h.RandString(10))
						exportArgs = append(exportArgs, exportedImageName)

						output := h.DockerRun(t,
							exportImage,
							h.WithFlags(
								"--env", "CNB_PLATFORM_API="+platformAPI,
								"--env", "CNB_REGISTRY_AUTH="+exportRegAuthConfig,
								"--network", exportRegNetwork,
							),
							h.WithArgs(exportArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+exportedImageName)

						h.Run(t, exec.Command("docker", "pull", exportedImageName))                              // TODO: cleanup this image
						inspect, _, err := h.DockerCli(t).ImageInspectWithRaw(context.TODO(), exportedImageName) // TODO: make test helper
						h.AssertNil(t, err)
						h.AssertEq(t, inspect.Os, exportTest.targetDaemon.os)
						h.AssertEq(t, inspect.Architecture, exportTest.targetDaemon.arch)
					})
				})
				when("cache", func() {
					when("cache image case", func() {
						it("is created", func() {
							cacheImageName := exportTest.targetRegistry.registry.RepoName("some-cache-image-" + h.RandString(10))
							exportFlags := []string{"-cache-image", cacheImageName}
							if api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0 {
								exportFlags = append(exportFlags, "-run-image", exportRegFixtures.ReadOnlyRunImage)
							}

							exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
							exportedImageName := exportTest.targetRegistry.registry.RepoName("some-exported-image-" + h.RandString(10))
							exportArgs = append(exportArgs, exportedImageName)

							output := h.DockerRun(t,
								exportImage,
								h.WithFlags(
									"--env", "CNB_PLATFORM_API="+platformAPI,
									"--env", "CNB_REGISTRY_AUTH="+exportRegAuthConfig,
									"--network", exportRegNetwork,
								),
								h.WithArgs(exportArgs...),
							)
							h.AssertStringContains(t, output, "Saving "+exportedImageName)

							h.Run(t, exec.Command("docker", "pull", exportedImageName))                              // TODO: cleanup this image
							inspect, _, err := h.DockerCli(t).ImageInspectWithRaw(context.TODO(), exportedImageName) // TODO: make test helper
							h.AssertNil(t, err)
							h.AssertEq(t, inspect.Os, exportTest.targetDaemon.os)
							h.AssertEq(t, inspect.Architecture, exportTest.targetDaemon.arch)

							// TODO: create issue for this maybe
							//h.Run(t, exec.Command("docker", "pull", cacheImageName))                                // TODO: cleanup this image
							//inspect, _, err = h.DockerCli(t).ImageInspectWithRaw(context.TODO(), cacheImageName) // TODO: make test helper
							//h.AssertNil(t, err)
							//h.AssertEq(t, inspect.Os, exportTest.targetDaemon.os)
							//h.AssertEq(t, inspect.Architecture, exportTest.targetDaemon.arch)
						})
					})
				})
			})
		})
	}
}

func modifyAnalyzedTOMLWithRegRepoName(t *testing.T, daemonFixtures *daemonImageFixtures, regFixtures *regImageFixtures) {
	// TODO: see about ignoring changes to *analyzed.toml
	analyzedPath := filepath.Join("testdata", "exporter", "container", "layers", "analyzed.toml")
	analyzedMD := assertAnalyzedMetadata(t, analyzedPath)
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: regFixtures.ReadOnlyRunImage} // TODO: check if metadata on fixture matches metadata in analyzed.toml
	lifecycle.WriteTOML(analyzedPath, analyzedMD)

	analyzedPath = filepath.Join("testdata", "exporter", "container", "layers", "daemon-analyzed.toml")
	analyzedMD = assertAnalyzedMetadata(t, analyzedPath)
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: daemonFixtures.RunImage} // TODO: check if metadata on fixture matches metadata in analyzed.toml
	lifecycle.WriteTOML(analyzedPath, analyzedMD)

	analyzedPath = filepath.Join("testdata", "exporter", "container", "layers", "some-analyzed.toml")
	analyzedMD = assertAnalyzedMetadata(t, analyzedPath)
	analyzedMD.Image = &platform.ImageIdentifier{Reference: regFixtures.SomeAppImage}        // TODO: check if metadata on fixture matches metadata in analyzed.toml
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: regFixtures.ReadOnlyRunImage} // TODO: check if metadata on fixture matches metadata in analyzed.toml
	lifecycle.WriteTOML(analyzedPath, analyzedMD)
}
