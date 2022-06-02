//go:build acceptance
// +build acceptance

package acceptance

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/imgutil"

	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/internal/encoding"
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

	for _, platformAPI := range platform.APIs.Supported {
		spec.Run(t, "acceptance-exporter/"+platformAPI.String(), testExporterFunc(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testExporterFunc(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var exportedImageName string

		it.After(func() {
			_, _, _ = h.RunE(exec.Command("docker", "rmi", exportedImageName)) // #nosec G204
		})

		when("daemon case", func() {
			when("first build", func() {
				when("app", func() {
					it("is created", func() {
						exportFlags := []string{"-daemon"}
						if api.MustParse(platformAPI).LessThan("0.7") {
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

						assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, imgutil.NormalizedDateTime)
					})
				})
			})
			when("SOURCE_DATE_EPOCH is set", func() {
				it("Image CreatedAt is set to SOURCE_DATE_EPOCH", func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.9"), "SOURCE_DATE_EPOCH support added in 0.9")
					expectedTime := time.Date(2022, 1, 5, 5, 5, 5, 0, time.UTC)

					exportFlags := []string{"-daemon"}
					if api.MustParse(platformAPI).LessThan("0.7") {
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
							"--env", "SOURCE_DATE_EPOCH="+fmt.Sprintf("%d", expectedTime.Unix()),
							"--network", exportRegNetwork,
						)...),
						h.WithArgs(exportArgs...),
					)
					h.AssertStringContains(t, output, "Saving "+exportedImageName)

					assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, expectedTime)
				})
			})
		})

		when("registry case", func() {
			when("first build", func() {
				when("app", func() {
					it("is created", func() {
						var exportFlags []string
						if api.MustParse(platformAPI).LessThan("0.7") {
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
						assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, imgutil.NormalizedDateTime)
					})
				})
				when("SOURCE_DATE_EPOCH is set", func() {
					it("Image CreatedAt is set to SOURCE_DATE_EPOCH", func() {
						h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.9"), "SOURCE_DATE_EPOCH support added in 0.9")
						expectedTime := time.Date(2022, 1, 5, 5, 5, 5, 0, time.UTC)

						var exportFlags []string
						if api.MustParse(platformAPI).LessThan("0.7") {
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
								"--env", "SOURCE_DATE_EPOCH="+fmt.Sprintf("%d", expectedTime.Unix()),
								"--network", exportRegNetwork,
							),
							h.WithArgs(exportArgs...),
						)
						h.AssertStringContains(t, output, "Saving "+exportedImageName)

						h.Run(t, exec.Command("docker", "pull", exportedImageName))
						assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, expectedTime)
					})
				})
				when("cache", func() {
					when("cache image case", func() {
						it("is created", func() {
							cacheImageName := exportTest.RegRepoName("some-cache-image-" + h.RandString(10))
							exportFlags := []string{"-cache-image", cacheImageName}
							if api.MustParse(platformAPI).LessThan("0.7") {
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
							assertImageOSAndArchAndCreatedAt(t, exportedImageName, exportTest, imgutil.NormalizedDateTime)
						})

						it("is created with empty layer", func() {
							cacheImageName := exportTest.RegRepoName("some-empty-cache-image-" + h.RandString(10))
							exportFlags := []string{"-cache-image", cacheImageName, "-layers", "/other_layers"}
							if api.MustParse(platformAPI).LessThan("0.7") {
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

							testEmptyLayerSHA := calculateEmptyLayerSha(t)

							// Retrieve the cache image from the ephemeral registry
							h.Run(t, exec.Command("docker", "pull", cacheImageName))
							subject, err := cache.NewImageCacheFromName(cacheImageName, authn.DefaultKeychain)
							h.AssertNil(t, err)

							//Assert the cache image was created with an empty layer
							layer, err := subject.RetrieveLayer(testEmptyLayerSHA)
							h.AssertNil(t, err)
							defer layer.Close()
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

func assertImageOSAndArchAndCreatedAt(t *testing.T, imageName string, phaseTest *PhaseTest, expectedCreatedAt time.Time) {
	inspect, _, err := h.DockerCli(t).ImageInspectWithRaw(context.TODO(), imageName)
	h.AssertNil(t, err)
	h.AssertEq(t, inspect.Os, phaseTest.targetDaemon.os)
	h.AssertEq(t, inspect.Architecture, phaseTest.targetDaemon.arch)
	h.AssertEq(t, inspect.Created, expectedCreatedAt.Format(time.RFC3339))
}

func updateAnalyzedTOMLFixturesWithRegRepoName(t *testing.T, phaseTest *PhaseTest) {
	placeHolderPath := filepath.Join("testdata", "exporter", "container", "layers", "analyzed.toml.placeholder")
	analyzedMD := assertAnalyzedMetadata(t, placeHolderPath)
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: phaseTest.targetRegistry.fixtures.ReadOnlyRunImage}
	encoding.WriteTOML(strings.TrimSuffix(placeHolderPath, ".placeholder"), analyzedMD)

	placeHolderPath = filepath.Join("testdata", "exporter", "container", "layers", "some-analyzed.toml.placeholder")
	analyzedMD = assertAnalyzedMetadata(t, placeHolderPath)
	analyzedMD.PreviousImage = &platform.ImageIdentifier{Reference: phaseTest.targetRegistry.fixtures.SomeAppImage}
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: phaseTest.targetRegistry.fixtures.ReadOnlyRunImage}
	encoding.WriteTOML(strings.TrimSuffix(placeHolderPath, ".placeholder"), analyzedMD)

	placeHolderPath = filepath.Join("testdata", "exporter", "container", "other_layers", "analyzed.toml.placeholder")
	analyzedMD = assertAnalyzedMetadata(t, placeHolderPath)
	analyzedMD.RunImage = &platform.ImageIdentifier{Reference: phaseTest.targetRegistry.fixtures.ReadOnlyRunImage}
	encoding.WriteTOML(strings.TrimSuffix(placeHolderPath, ".placeholder"), analyzedMD)
}

func calculateEmptyLayerSha(t *testing.T) string {
	tmpDir, err := ioutil.TempDir("", "")
	h.AssertNil(t, err)
	testLayerEmptyPath := filepath.Join(tmpDir, "empty.tar")
	h.AssertNil(t, ioutil.WriteFile(testLayerEmptyPath, []byte{}, 0600))
	return "sha256:" + h.ComputeSHA256ForFile(t, testLayerEmptyPath)
}
