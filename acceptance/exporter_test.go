// +build acceptance

package acceptance

import (
	"context"
	"math/rand"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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
	exportTest.Start(t, updateAnalyzedTOMLFixturesWithRegRepoName)
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
		var exportedImageName string

		it.After(func() {
			h.DockerImageRemove(t, exportedImageName)
		})

		when("daemon case", func() {
			when("first build", func() {
				when("app", func() {
					it("is created", func() {
						exportFlags := []string{"-daemon"}
						if api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0 {
							exportFlags = append(exportFlags, []string{"-run-image", exportRegFixtures.ReadOnlyRunImage}...)
						}

						exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
						exportedImageName = "some-exported-image-" + h.RandString(10)
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

						assertImageOSAndArch(t, exportedImageName, exportTest)
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
						exportedImageName = exportTest.RegRepoName("some-exported-image-" + h.RandString(10))
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

						h.Run(t, exec.Command("docker", "pull", exportedImageName))
						assertImageOSAndArch(t, exportedImageName, exportTest)
					})
				})
				when("cache", func() {
					when("cache image case", func() {
						it("is created", func() {
							cacheImageName := exportTest.RegRepoName("some-cache-image-" + h.RandString(10))
							exportFlags := []string{"-cache-image", cacheImageName}
							if api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0 {
								exportFlags = append(exportFlags, "-run-image", exportRegFixtures.ReadOnlyRunImage)
							}

							exportArgs := append([]string{ctrPath(exporterPath)}, exportFlags...)
							exportedImageName = exportTest.RegRepoName("some-exported-image-" + h.RandString(10))
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

							h.Run(t, exec.Command("docker", "pull", exportedImageName))
							assertImageOSAndArch(t, exportedImageName, exportTest)
						})
					})
				})
			})
		})
	}
}

func assertImageOSAndArch(t *testing.T, imageName string, phaseTest *PhaseTest) {
	inspect, _, err := h.DockerCli(t).ImageInspectWithRaw(context.TODO(), imageName)
	h.AssertNil(t, err)
	h.AssertEq(t, inspect.Os, phaseTest.targetDaemon.os)
	h.AssertEq(t, inspect.Architecture, phaseTest.targetDaemon.arch)
}

func updateAnalyzedTOMLFixturesWithRegRepoName(t *testing.T, phaseTest *PhaseTest) {
	placeHolderPath := filepath.Join("testdata", "exporter", "container", "layers", "analyzed.toml.placeholder")
	analyzedMD := assertAnalyzedMetadata(t, placeHolderPath)
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: phaseTest.targetRegistry.fixtures.ReadOnlyRunImage}
	lifecycle.WriteTOML(strings.TrimSuffix(placeHolderPath, ".placeholder"), analyzedMD)

	placeHolderPath = filepath.Join("testdata", "exporter", "container", "layers", "some-analyzed.toml.placeholder")
	analyzedMD = assertAnalyzedMetadata(t, placeHolderPath)
	analyzedMD.Image = &platform.ImageIdentifier{Reference: phaseTest.targetRegistry.fixtures.SomeAppImage}
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: phaseTest.targetRegistry.fixtures.ReadOnlyRunImage}
	lifecycle.WriteTOML(strings.TrimSuffix(placeHolderPath, ".placeholder"), analyzedMD)
}
