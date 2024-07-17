package phase_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/phase/testmock"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestAnalyzer(t *testing.T) {
	spec.Run(t, "unit-new-analyzer/", testAnalyzerFactory, spec.Parallel(), spec.Report(report.Terminal{}))
	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "unit-analyzer/"+platformAPI.String(), testAnalyzer(platformAPI.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testAnalyzerFactory(t *testing.T, when spec.G, it spec.S) {
	when("#NewAnalyzer", func() {
		var (
			analyzerFactory     *phase.ConnectedFactory
			fakeAPIVerifier     *testmock.MockBuildpackAPIVerifier
			fakeCacheHandler    *testmock.MockCacheHandler
			fakeConfigHandler   *testmock.MockConfigHandler
			fakeImageHandler    *testmock.MockHandler
			fakeRegistryHandler *testmock.MockRegistryHandler
			logger              *log.Logger
			mockController      *gomock.Controller
			tempDir             string
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			fakeAPIVerifier = testmock.NewMockBuildpackAPIVerifier(mockController)
			fakeCacheHandler = testmock.NewMockCacheHandler(mockController)
			fakeConfigHandler = testmock.NewMockConfigHandler(mockController)
			fakeImageHandler = testmock.NewMockHandler(mockController)
			fakeRegistryHandler = testmock.NewMockRegistryHandler(mockController)
			logger = &log.Logger{Handler: &discard.Handler{}}
			var err error
			tempDir, err = os.MkdirTemp("", "")
			h.AssertNil(t, err)
		})

		it.After(func() {
			mockController.Finish()
			_ = os.RemoveAll(tempDir)
		})

		when("platform api >= 0.8", func() {
			it.Before(func() {
				analyzerFactory = phase.NewConnectedFactory(
					api.Platform.Latest(),
					fakeAPIVerifier,
					fakeCacheHandler,
					fakeConfigHandler,
					fakeImageHandler,
					fakeRegistryHandler,
				)
			})

			when("layout case", func() {
				it("configures the analyzer", func() {
					previousImage := fakes.NewImage("some-previous-image-ref", "", nil)
					runImage := fakes.NewImage("some-run-image-ref", "", nil)

					t.Log("ensures registry access")
					fakeImageHandler.EXPECT().Kind().Return(image.LayoutKind).AnyTimes()
					// Only caching must be checked for writing access
					fakeRegistryHandler.EXPECT().EnsureWriteAccess([]string{"some-cache-image-ref"})
					// we don't expect any read access check when -layout is used
					fakeRegistryHandler.EXPECT().EnsureReadAccess([]string{})

					t.Log("does not process cache")

					t.Log("processes previous image")
					fakeImageHandler.EXPECT().InitImage("some-previous-image-ref").Return(previousImage, nil)

					t.Log("processes run image")
					fakeImageHandler.EXPECT().InitImage("some-run-image-ref").Return(runImage, nil)

					analyzer, err := analyzerFactory.NewAnalyzer(platform.LifecycleInputs{
						AdditionalTags:   []string{"some-additional-tag"},
						CacheImageRef:    "some-cache-image-ref",
						LaunchCacheDir:   "some-launch-cache-dir",
						LayersDir:        "some-layers-dir",
						OutputImageRef:   "some-output-image-ref",
						PreviousImageRef: "some-previous-image-ref",
						RunImageRef:      "some-run-image-ref",
						SkipLayers:       false,
					}, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, analyzer.PreviousImage.Name(), previousImage.Name())
					h.AssertEq(t, analyzer.RunImage.Name(), runImage.Name())

					t.Log("restores sbom data")
					sbomRestorer, ok := analyzer.SBOMRestorer.(*layer.DefaultSBOMRestorer)
					h.AssertEq(t, ok, true)
					h.AssertEq(t, sbomRestorer.LayersDir, "some-layers-dir")
					h.AssertEq(t, sbomRestorer.Logger, logger)

					t.Log("sets logger")
					h.AssertEq(t, analyzer.Logger, logger)
				})
			})

			it("configures the analyzer", func() {
				previousImage := fakes.NewImage("some-previous-image-ref", "", nil)
				runImage := fakes.NewImage("some-run-image-ref", "", nil)

				t.Log("ensures registry access")
				fakeImageHandler.EXPECT().Kind().Return(image.RemoteKind).AnyTimes()
				fakeRegistryHandler.EXPECT().EnsureReadAccess([]string{"some-previous-image-ref", "some-run-image-ref"})
				fakeRegistryHandler.EXPECT().EnsureWriteAccess([]string{"some-cache-image-ref", "some-output-image-ref", "some-additional-tag"})

				t.Log("does not process cache")

				t.Log("processes previous image")
				fakeImageHandler.EXPECT().InitImage("some-previous-image-ref").Return(previousImage, nil)

				t.Log("processes run image")
				fakeImageHandler.EXPECT().InitImage("some-run-image-ref").Return(runImage, nil)

				analyzer, err := analyzerFactory.NewAnalyzer(platform.LifecycleInputs{
					AdditionalTags:   []string{"some-additional-tag"},
					CacheImageRef:    "some-cache-image-ref",
					LaunchCacheDir:   "some-launch-cache-dir",
					LayersDir:        "some-layers-dir",
					OutputImageRef:   "some-output-image-ref",
					PreviousImageRef: "some-previous-image-ref",
					RunImageRef:      "some-run-image-ref",
					SkipLayers:       false,
				}, logger)
				h.AssertNil(t, err)
				h.AssertEq(t, analyzer.PreviousImage.Name(), previousImage.Name())
				h.AssertEq(t, analyzer.RunImage.Name(), runImage.Name())

				t.Log("restores sbom data")
				sbomRestorer, ok := analyzer.SBOMRestorer.(*layer.DefaultSBOMRestorer)
				h.AssertEq(t, ok, true)
				h.AssertEq(t, sbomRestorer.LayersDir, "some-layers-dir")
				h.AssertEq(t, sbomRestorer.Logger, logger)
				h.AssertEq(t, analyzer.PlatformAPI, api.Platform.Latest())

				t.Log("sets logger")
				h.AssertEq(t, analyzer.Logger, logger)
			})

			when("daemon case", func() {
				it("configures the analyzer", func() {
					previousImage := fakes.NewImage("some-previous-image-ref", "", nil)
					runImage := fakes.NewImage("some-run-image-ref", "", nil)

					t.Log("ensures registry access")
					fakeImageHandler.EXPECT().Kind().Return(image.LocalKind).AnyTimes()
					fakeRegistryHandler.EXPECT().EnsureReadAccess()
					fakeRegistryHandler.EXPECT().EnsureWriteAccess([]string{"some-cache-image-ref"})

					t.Log("processes previous image")
					fakeImageHandler.EXPECT().InitImage("some-previous-image-ref").Return(previousImage, nil)

					t.Log("processes run image")
					fakeImageHandler.EXPECT().InitImage("some-run-image-ref").Return(runImage, nil)

					launchCacheDir := filepath.Join(tempDir, "some-launch-cache-dir")
					h.AssertNil(t, os.MkdirAll(launchCacheDir, 0777))
					analyzer, err := analyzerFactory.NewAnalyzer(platform.LifecycleInputs{
						AdditionalTags:   []string{"some-additional-tag"},
						CacheImageRef:    "some-cache-image-ref",
						LaunchCacheDir:   launchCacheDir,
						LayersDir:        "some-layers-dir",
						OutputImageRef:   "some-output-image-ref",
						PreviousImageRef: "some-previous-image-ref",
						RunImageRef:      "some-run-image-ref",
						SkipLayers:       false,
					}, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, analyzer.PreviousImage.Name(), previousImage.Name())
					h.AssertEq(t, analyzer.RunImage.Name(), runImage.Name())

					t.Log("uses the provided launch cache")
					_, ok := analyzer.PreviousImage.(*cache.CachingImage)
					h.AssertEq(t, ok, true)
					h.AssertPathExists(t, filepath.Join(launchCacheDir, "committed"))
					h.AssertPathExists(t, filepath.Join(launchCacheDir, "staging"))
				})
			})

			when("skip layers", func() {
				it("does not restore sbom data", func() {
					fakeImageHandler.EXPECT().Kind().Return(image.RemoteKind).AnyTimes()
					fakeRegistryHandler.EXPECT().EnsureReadAccess(gomock.Any())
					fakeRegistryHandler.EXPECT().EnsureWriteAccess(gomock.Any())
					fakeImageHandler.EXPECT().InitImage(gomock.Any())
					fakeImageHandler.EXPECT().InitImage(gomock.Any())

					analyzer, err := analyzerFactory.NewAnalyzer(platform.LifecycleInputs{
						AdditionalTags:   []string{"some-additional-tag"},
						CacheImageRef:    "some-cache-image-ref",
						LaunchCacheDir:   "some-launch-cache-dir",
						LayersDir:        "some-layers-dir",
						OutputImageRef:   "some-output-image-ref",
						PreviousImageRef: "some-previous-image-ref",
						RunImageRef:      "some-run-image-ref",
						SkipLayers:       true,
					}, logger)
					h.AssertNil(t, err)

					_, ok := analyzer.SBOMRestorer.(*layer.NopSBOMRestorer)
					h.AssertEq(t, ok, true)
				})
			})
		})

		when("platform api = 0.7", func() {
			it.Before(func() {
				analyzerFactory = phase.NewConnectedFactory(
					api.MustParse("0.7"),
					fakeAPIVerifier,
					fakeCacheHandler,
					fakeConfigHandler,
					fakeImageHandler,
					fakeRegistryHandler,
				)
			})

			it("configures the analyzer", func() {
				previousImage := fakes.NewImage("some-previous-image-ref", "", nil)
				runImage := fakes.NewImage("some-run-image-ref", "", nil)

				t.Log("ensures registry access")
				fakeImageHandler.EXPECT().Kind().Return(image.RemoteKind).AnyTimes()
				fakeRegistryHandler.EXPECT().EnsureReadAccess([]string{"some-previous-image-ref", "some-run-image-ref"})
				fakeRegistryHandler.EXPECT().EnsureWriteAccess([]string{"some-cache-image-ref", "some-output-image-ref", "some-additional-tag"})

				t.Log("processes previous image")
				fakeImageHandler.EXPECT().InitImage("some-previous-image-ref").Return(previousImage, nil)

				t.Log("processes run image")
				fakeImageHandler.EXPECT().InitImage("some-run-image-ref").Return(runImage, nil)

				analyzer, err := analyzerFactory.NewAnalyzer(platform.LifecycleInputs{
					AdditionalTags:   []string{"some-additional-tag"},
					CacheImageRef:    "some-cache-image-ref",
					LaunchCacheDir:   "some-launch-cache-dir",
					LayersDir:        "some-layers-dir",
					OutputImageRef:   "some-output-image-ref",
					PreviousImageRef: "some-previous-image-ref",
					RunImageRef:      "some-run-image-ref",
					SkipLayers:       true,
				}, logger)
				h.AssertNil(t, err)
				h.AssertEq(t, analyzer.PreviousImage.Name(), previousImage.Name())
				h.AssertEq(t, analyzer.RunImage.Name(), runImage.Name())

				t.Log("does not restore sbom data")
				_, ok := analyzer.SBOMRestorer.(*layer.NopSBOMRestorer)
				h.AssertEq(t, ok, true)

				t.Log("sets logger")
				h.AssertEq(t, analyzer.Logger, logger)
			})

			when("daemon case", func() {
				it("configures the analyzer", func() {
					previousImage := fakes.NewImage("some-previous-image-ref", "", nil)
					runImage := fakes.NewImage("some-run-image-ref", "", nil)

					t.Log("ensures registry access")
					fakeImageHandler.EXPECT().Kind().Return(image.LocalKind).AnyTimes()
					fakeRegistryHandler.EXPECT().EnsureReadAccess()
					fakeRegistryHandler.EXPECT().EnsureWriteAccess([]string{"some-cache-image-ref"})

					t.Log("processes previous image")
					fakeImageHandler.EXPECT().InitImage("some-previous-image-ref").Return(previousImage, nil)

					t.Log("processes run image")
					fakeImageHandler.EXPECT().InitImage("some-run-image-ref").Return(runImage, nil)

					launchCacheDir := filepath.Join(tempDir, "some-launch-cache-dir")
					h.AssertNil(t, os.MkdirAll(launchCacheDir, 0777))
					analyzer, err := analyzerFactory.NewAnalyzer(platform.LifecycleInputs{
						AdditionalTags:   []string{"some-additional-tag"},
						CacheImageRef:    "some-cache-image-ref",
						LaunchCacheDir:   launchCacheDir,
						LayersDir:        "some-layers-dir",
						OutputImageRef:   "some-output-image-ref",
						PreviousImageRef: "some-previous-image-ref",
						RunImageRef:      "some-run-image-ref",
						SkipLayers:       true,
					}, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, analyzer.PreviousImage.Name(), previousImage.Name())
					h.AssertEq(t, analyzer.RunImage.Name(), runImage.Name())
				})
			})
		})
	})
}

func testAnalyzer(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var (
			cacheDir      string
			layersDir     string
			tmpDir        string
			analyzer      *phase.Analyzer
			previousImage *fakes.Image
			mockCtrl      *gomock.Controller
			sbomRestorer  *testmock.MockSBOMRestorer
			testCache     phase.Cache
		)

		it.Before(func() {
			var err error
			discardLogger := log.Logger{Handler: &discard.Handler{}}

			tmpDir, err = os.MkdirTemp("", "analyzer-tests")
			h.AssertNil(t, err)

			layersDir, err = os.MkdirTemp("", "lifecycle-layer-dir")
			h.AssertNil(t, err)

			cacheDir, err = os.MkdirTemp("", "some-cache-dir")
			h.AssertNil(t, err)

			testCache, err = cache.NewVolumeCache(cacheDir, &discardLogger)
			h.AssertNil(t, err)

			previousImage = fakes.NewImage("image-repo-name", "", local.IDIdentifier{
				ImageID: "s0m3D1g3sT",
			})

			mockCtrl = gomock.NewController(t)

			sbomRestorer = testmock.NewMockSBOMRestorer(mockCtrl)

			h.AssertNil(t, err)
			analyzer = &phase.Analyzer{
				PreviousImage: previousImage,
				Logger:        &discardLogger,
				SBOMRestorer:  sbomRestorer,
				PlatformAPI:   api.MustParse(platformAPI),
			}

			if testing.Verbose() {
				analyzer.Logger = cmd.DefaultLogger
				h.AssertNil(t, cmd.DefaultLogger.SetLevel("debug"))
			}
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tmpDir))
			h.AssertNil(t, os.RemoveAll(layersDir))
			h.AssertNil(t, os.RemoveAll(cacheDir))
			h.AssertNil(t, previousImage.Cleanup())
			mockCtrl.Finish()
		})

		when("#Analyze", func() {
			var (
				expectedAppMetadata   files.LayersMetadata
				expectedCacheMetadata platform.CacheMetadata
				ref                   *testmock.MockReference
			)

			it.Before(func() {
				ref = testmock.NewMockReference(mockCtrl)
				ref.EXPECT().Name().AnyTimes()
			})

			when("previous image exists", func() {
				it.Before(func() {
					metadata := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "app_metadata.json"))
					h.AssertNil(t, previousImage.SetLabel("io.buildpacks.lifecycle.metadata", string(metadata)))
					h.AssertNil(t, json.Unmarshal(metadata, &expectedAppMetadata))
				})

				it("returns the analyzed metadata", func() {
					md, err := analyzer.Analyze()
					h.AssertNil(t, err)

					h.AssertEq(t, md.PreviousImageRef(), "s0m3D1g3sT")
					h.AssertEq(t, md.LayersMetadata, expectedAppMetadata)
				})

				when("cache exists", func() {
					it.Before(func() {
						metadata := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "cache_metadata.json"))
						h.AssertNil(t, json.Unmarshal(metadata, &expectedCacheMetadata))
						h.AssertNil(t, testCache.SetMetadata(expectedCacheMetadata))
						h.AssertNil(t, testCache.Commit())
					})

					it("returns the analyzed metadata", func() {
						md, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertEq(t, md.LayersMetadata, expectedAppMetadata)
					})
				})
			})

			when("previous image not found", func() {
				it.Before(func() {
					h.AssertNil(t, previousImage.Delete())
				})

				it("returns a nil image in the analyzed metadata", func() {
					md, err := analyzer.Analyze()
					h.AssertNil(t, err)

					h.AssertEq(t, md.PreviousImageRef(), "")
					h.AssertEq(t, md.LayersMetadata, files.LayersMetadata{})
				})
			})

			when("previous image does not have metadata label", func() {
				it.Before(func() {
					h.AssertNil(t, previousImage.SetLabel("io.buildpacks.lifecycle.metadata", ""))
				})

				it("returns empty analyzed metadata", func() {
					md, err := analyzer.Analyze()
					h.AssertNil(t, err)
					h.AssertEq(t, md.LayersMetadata, files.LayersMetadata{})
				})
			})

			when("previous image has incompatible metadata", func() {
				it.Before(func() {
					h.AssertNil(t, previousImage.SetLabel("io.buildpacks.lifecycle.metadata", `{["bad", "metadata"]}`))
				})

				it("returns empty analyzed metadata", func() {
					md, err := analyzer.Analyze()
					h.AssertNil(t, err)
					h.AssertEq(t, md.LayersMetadata, files.LayersMetadata{})
				})
			})

			when("previous image has an SBOM layer digest in the analyzed metadata", func() {
				it.Before(func() {
					metadata := fmt.Sprintf(`{"sbom": {"sha":"%s"}}`, "some-digest")
					h.AssertNil(t, previousImage.SetLabel("io.buildpacks.lifecycle.metadata", metadata))
					h.AssertNil(t, json.Unmarshal([]byte(metadata), &expectedAppMetadata))
				})

				it("calls the SBOM restorer with the SBOM layer digest", func() {
					sbomRestorer.EXPECT().RestoreFromPrevious(previousImage, "some-digest")
					_, err := analyzer.Analyze()
					h.AssertNil(t, err)
				})
			})

			when("run image is provided", func() {
				it.Before(func() {
					analyzer.RunImage = previousImage
				})

				it("returns the run image digest in the analyzed metadata", func() {
					md, err := analyzer.Analyze()
					h.AssertNil(t, err)

					h.AssertEq(t, md.RunImage.Reference, "s0m3D1g3sT")
				})

				it("populates target metadata from the run image", func() {
					h.AssertNil(t, previousImage.SetLabel("io.buildpacks.base.id", "id software"))
					h.AssertNil(t, previousImage.SetOS("windows"))
					h.AssertNil(t, previousImage.SetOSVersion("95"))
					h.AssertNil(t, previousImage.SetArchitecture("Pentium"))
					h.AssertNil(t, previousImage.SetVariant("MMX"))
					h.AssertNil(t, previousImage.SetLabel("io.buildpacks.base.distro.name", "moobuntu"))
					h.AssertNil(t, previousImage.SetLabel("io.buildpacks.base.distro.version", "Helpful Holstein"))

					md, err := analyzer.Analyze()
					h.AssertNil(t, err)
					if api.MustParse(platformAPI).LessThan("0.12") {
						h.AssertNil(t, md.RunImage.TargetMetadata)
					} else {
						h.AssertNotNil(t, md.RunImage.TargetMetadata)
						h.AssertEq(t, md.RunImage.TargetMetadata.Arch, "Pentium")
						h.AssertEq(t, md.RunImage.TargetMetadata.ArchVariant, "MMX")
						h.AssertEq(t, md.RunImage.TargetMetadata.OS, "windows")
						h.AssertEq(t, md.RunImage.TargetMetadata.ID, "id software")
						h.AssertNotNil(t, md.RunImage.TargetMetadata.Distro)
						h.AssertEq(t, md.RunImage.TargetMetadata.Distro.Name, "moobuntu")
						h.AssertEq(t, md.RunImage.TargetMetadata.Distro.Version, "Helpful Holstein")
					}
				})

				when("run image is missing OS", func() {
					it("errors", func() {
						h.AssertNil(t, previousImage.SetOS(""))
						_, err := analyzer.Analyze()
						if api.MustParse(platformAPI).LessThan("0.12") {
							h.AssertNil(t, err)
						} else {
							h.AssertError(t, err, "failed to find OS")
						}
					})
				})
			})
		})
	}
}
