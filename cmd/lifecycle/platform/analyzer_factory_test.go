package platform_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/apex/log/handlers/memory"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/platform"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/platform/testmock"
	"github.com/buildpacks/lifecycle/internal/layer"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestAnalyzerFactory(t *testing.T) {
	spec.Run(t, "unit-analyzer-ops-manager", testAnalyzerOpsManager, spec.Parallel(), spec.Report(report.Terminal{}))
	for _, api := range api.Platform.Supported {
		spec.Run(t, "unit-analyzer-factory/"+api.String(), testAnalyzerFactory(api.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testAnalyzerOpsManager(t *testing.T, when spec.G, it spec.S) {
	var (
		om                    *platform.DefaultAnalyzerOpsManager
		fakeCacheHandler      *testmock.MockCacheHandler
		fakeImageHandler      *testmock.MockImageHandler
		fakeRegistryValidator *testmock.MockRegistryValidator
		logHandler            *memory.Handler
		logger                *log.Logger
		mockController        *gomock.Controller
		tempDir               string
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
		fakeCacheHandler = testmock.NewMockCacheHandler(mockController)
		fakeImageHandler = testmock.NewMockImageHandler(mockController)
		fakeRegistryValidator = testmock.NewMockRegistryValidator(mockController)
		om = &platform.DefaultAnalyzerOpsManager{
			CacheHandler:      fakeCacheHandler,
			ImageHandler:      fakeImageHandler,
			RegistryValidator: fakeRegistryValidator,
		}
		logHandler = memory.New()
		logger = &log.Logger{Handler: logHandler}
		var err error
		tempDir, err = ioutil.TempDir("", "")
		h.AssertNil(t, err)
		h.AssertNil(t, os.Mkdir(filepath.Join(tempDir, "launch-cache"), 0755))
		h.AssertNil(t, os.Mkdir(filepath.Join(tempDir, "cache"), 0755))
	})

	it.After(func() {
		mockController.Finish()
		os.RemoveAll(tempDir)
	})

	when("EnsureRegistryAccess", func() {
		when("exporting to a daemon", func() {
			it.Before(func() {
				fakeImageHandler.EXPECT().Docker().Return(true).AnyTimes()
				fakeImageHandler.EXPECT().InitImage(gomock.Any()).AnyTimes()
			})

			it("validates access", func() {
				opts := platform.AnalyzerOpts{
					AdditionalTags:   []string{"some-additional-tag"},
					CacheImageRef:    "some-cache-image-ref",
					OutputImageRef:   "some-output-image-ref",
					PreviousImageRef: "some-previous-image-ref",
					RunImageRef:      "some-run-image-ref",
				}
				var none []string
				fakeRegistryValidator.EXPECT().ValidateReadAccess(none)
				fakeRegistryValidator.EXPECT().ValidateWriteAccess([]string{"some-cache-image-ref"})

				h.AssertNil(t, om.EnsureRegistryAccess(opts)(&lifecycle.Analyzer{}))
			})
		})

		when("exporting to a registry", func() {
			it.Before(func() {
				fakeImageHandler.EXPECT().Docker().Return(false).AnyTimes()
				fakeImageHandler.EXPECT().InitImage(gomock.Any()).AnyTimes()
			})

			it("validates access", func() {
				opts := platform.AnalyzerOpts{
					AdditionalTags:   []string{"some-additional-tag"},
					CacheImageRef:    "some-cache-image-ref",
					OutputImageRef:   "some-output-image-ref",
					PreviousImageRef: "some-previous-image-ref",
					RunImageRef:      "some-run-image-ref",
				}
				expectedReadImages := []string{
					"some-previous-image-ref",
					"some-run-image-ref",
				}
				expectedWriteImages := []string{
					"some-cache-image-ref",
					"some-output-image-ref",
					"some-additional-tag",
				}
				fakeRegistryValidator.EXPECT().ValidateReadAccess(expectedReadImages)
				fakeRegistryValidator.EXPECT().ValidateWriteAccess(expectedWriteImages)

				h.AssertNil(t, om.EnsureRegistryAccess(opts)(&lifecycle.Analyzer{}))
			})
		})
	})

	when("WithBuildpacks", func() {
		it("reads group.toml", func() {
			groupPath := filepath.Join("testdata", "layers", "group.toml")
			analyzer := &lifecycle.Analyzer{}

			h.AssertNil(t, om.WithBuildpacks(buildpack.Group{}, groupPath)(analyzer))
			h.AssertEq(t, analyzer.Buildpacks, []buildpack.GroupBuildpack{
				{ID: "some-buildpack-id", Version: "some-buildpack-version", API: "0.7", Homepage: "some-buildpack-homepage"},
			})
		})

		it("validates buildpack apis", func() {
			groupPath := filepath.Join("testdata", "layers", "bad-group.toml")
			analyzer := &lifecycle.Analyzer{}

			h.AssertNotNil(t, om.WithBuildpacks(buildpack.Group{}, groupPath)(analyzer))
		})
	})

	when("WithCache", func() {
		when("provided a cache image", func() {
			it("provides it to the analyzer", func() {
				origImage := fakes.NewImage("some-cache-image", "", nil)
				cacheImage := cache.NewImageCache(origImage, &fakes.Image{})
				fakeCacheHandler.EXPECT().InitImageCache("some-cache-image").Return(cacheImage, nil)
				analyzer := &lifecycle.Analyzer{}

				h.AssertNil(t, om.WithCache("some-cache-image", "")(analyzer))
				cacheImage, ok := analyzer.Cache.(*cache.ImageCache)
				h.AssertEq(t, ok, true)
				h.AssertEq(t, cacheImage.Name(), "some-cache-image")
			})
		})

		when("provided a cache directory", func() {
			it.Before(func() {
				om.CacheHandler = platform.NewCacheHandler(nil) // use a real cache handler
			})

			it("provides it to the analyzer", func() {
				analyzer := &lifecycle.Analyzer{}

				h.AssertNil(t, om.WithCache("", filepath.Join(tempDir, "cache"))(analyzer))
				cacheDir, ok := analyzer.Cache.(*cache.VolumeCache)
				h.AssertEq(t, ok, true)
				h.AssertEq(t, cacheDir.Name(), filepath.Join(tempDir, "cache"))
			})
		})
	})

	when("WithLayerMetadataRestorer", func() {
		it("provides a layer metadata restorer to the analyzer", func() {
			analyzer := &lifecycle.Analyzer{}

			h.AssertNil(t, om.WithLayerMetadataRestorer("some-layers-dir", false, logger)(analyzer))
			defaultRestorer, ok := analyzer.LayerMetadataRestorer.(*layer.DefaultMetadataRestorer)
			h.AssertEq(t, ok, true)
			h.AssertEq(t, defaultRestorer.LayersDir, "some-layers-dir")
		})
	})

	when("WithPrevious", func() {
		it("provides it to the analyzer", func() {
			previousImageRef := "some-previous-image-ref"
			previousImage := fakes.NewImage(previousImageRef, "", nil)
			fakeImageHandler.EXPECT().InitImage(previousImageRef).Return(previousImage, nil)
			analyzer := &lifecycle.Analyzer{}

			h.AssertNil(t, om.WithPrevious(previousImageRef, "")(analyzer))
			h.AssertEq(t, analyzer.PreviousImage.Name(), previousImageRef)
		})

		when("daemon case", func() {
			it.Before(func() {
				fakeImageHandler.EXPECT().Docker().Return(true).AnyTimes()
				fakeRegistryValidator.EXPECT().ValidateReadAccess(gomock.Any()).AnyTimes()
				fakeRegistryValidator.EXPECT().ValidateWriteAccess(gomock.Any()).AnyTimes()
			})

			when("provided a launch cache dir", func() {
				it("previous image is a caching image", func() {
					previousImageRef := "some-previous-image-ref"
					launchCacheDir := filepath.Join(tempDir, "launch-cache")
					previousImage := fakes.NewImage(previousImageRef, "", nil)
					fakeImageHandler.EXPECT().InitImage(previousImageRef).Return(previousImage, nil)
					analyzer := &lifecycle.Analyzer{}

					h.AssertNil(t, om.WithPrevious(previousImageRef, launchCacheDir)(analyzer))
					h.AssertEq(t, analyzer.PreviousImage.Name(), previousImageRef)
					_, ok := analyzer.PreviousImage.(*cache.CachingImage)
					h.AssertEq(t, ok, true)
					h.AssertPathExists(t, filepath.Join(tempDir, "launch-cache", "committed"))
					h.AssertPathExists(t, filepath.Join(tempDir, "launch-cache", "staging"))
				})
			})

			when("not provided a launch cache dir", func() {
				it("previous image is a regular image", func() {
					previousImageRef := "some-previous-image-ref"
					previousImage := fakes.NewImage(previousImageRef, "", nil)
					fakeImageHandler.EXPECT().InitImage(previousImageRef).Return(previousImage, nil)
					analyzer := &lifecycle.Analyzer{}

					h.AssertNil(t, om.WithPrevious(previousImageRef, "")(analyzer))
					h.AssertEq(t, analyzer.PreviousImage.Name(), previousImageRef)
					_, ok := analyzer.PreviousImage.(*fakes.Image)
					h.AssertEq(t, ok, true)
				})
			})
		})
	})

	when("WithRun", func() {
		it("provides it to the analyzer", func() {
			runImageRef := "some-run-image-ref"
			runImage := fakes.NewImage(runImageRef, "", nil)
			fakeImageHandler.EXPECT().InitImage(runImageRef).Return(runImage, nil)
			analyzer := &lifecycle.Analyzer{}

			h.AssertNil(t, om.WithRun(runImageRef)(analyzer))
			h.AssertEq(t, analyzer.RunImage.Name(), runImageRef)
		})
	})

	when("WithSBOMRestorer", func() {
		it("provides an sbom restorer to the analyzer", func() {
			analyzer := &lifecycle.Analyzer{}

			h.AssertNil(t, om.WithSBOMRestorer("some-layers-dir", logger)(analyzer))
			defaultRestorer, ok := analyzer.SBOMRestorer.(*layer.DefaultSBOMRestorer)
			h.AssertEq(t, ok, true)
			h.AssertEq(t, defaultRestorer.LayersDir, "some-layers-dir")
		})
	})
}

func testAnalyzerFactory(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var (
			af             *platform.AnalyzerFactory
			om             *testmock.MockAnalyzerOpsManager
			logger         *log.Logger
			callCount      int
			mockController *gomock.Controller
		)
		wasCalled := func(_ *lifecycle.Analyzer) error {
			callCount++
			return nil
		}

		it.Before(func() {
			mockController = gomock.NewController(t)
			om = testmock.NewMockAnalyzerOpsManager(mockController)
			af = &platform.AnalyzerFactory{
				PlatformAPI:        api.MustParse(platformAPI),
				AnalyzerOpsManager: om,
			}
			logger = &log.Logger{Handler: &discard.Handler{}}
		})

		it.After(func() {
			mockController.Finish()
		})

		it("provides platform and logger to the analyzer", func() {
			opts := platform.AnalyzerOpts{}
			om.EXPECT().EnsureRegistryAccess(opts).Return(wasCalled).AnyTimes()
			om.EXPECT().WithBuildpacks(opts.LegacyGroup, opts.LegacyGroupPath).Return(wasCalled).AnyTimes()
			om.EXPECT().WithCache(opts.CacheImageRef, opts.LegacyCacheDir).Return(wasCalled).AnyTimes()
			om.EXPECT().WithLayerMetadataRestorer(opts.LayersDir, opts.SkipLayers, logger).Return(wasCalled).AnyTimes()
			om.EXPECT().WithPrevious(opts.PreviousImageRef, opts.LaunchCacheDir).Return(wasCalled).AnyTimes()
			om.EXPECT().WithRun(opts.RunImageRef).Return(wasCalled).AnyTimes()
			om.EXPECT().WithSBOMRestorer(opts.LayersDir, logger).Return(wasCalled).AnyTimes()

			analyzer, err := af.NewAnalyzer(opts, logger)
			h.AssertNil(t, err)
			h.AssertEq(t, analyzer.Platform.API().String(), af.PlatformAPI.String())
			h.AssertEq(t, analyzer.Logger, logger)
		})

		when("latest platform api(s)", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.8"), "")
			})

			it("calls the expected operations", func() {
				opts := platform.AnalyzerOpts{
					LaunchCacheDir:   "some-launch-cache-dir",
					LayersDir:        "some-layers-dir",
					LegacyCacheDir:   "some-ignored-cache-dir",
					LegacyGroupPath:  "some-ignored-group.toml",
					PreviousImageRef: "some-previous-image",
					RunImageRef:      "some-run-image",
				}
				om.EXPECT().EnsureRegistryAccess(opts).Return(wasCalled)
				om.EXPECT().WithPrevious(opts.PreviousImageRef, opts.LaunchCacheDir).Return(wasCalled)
				om.EXPECT().WithRun(opts.RunImageRef).Return(wasCalled)
				om.EXPECT().WithSBOMRestorer(opts.LayersDir, logger).Return(wasCalled)

				_, err := af.NewAnalyzer(opts, logger)
				h.AssertNil(t, err)
				h.AssertEq(t, callCount, 4)
			})
		})

		when("platform api 0.7", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.8"), "")
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.7"), "")
			})

			it("calls the expected operations", func() {
				opts := platform.AnalyzerOpts{
					LayersDir:        "some-layers-dir",
					LegacyCacheDir:   "some-ignored-cache-dir",
					LegacyGroupPath:  "some-ignored-group.toml",
					PreviousImageRef: "some-previous-image",
					RunImageRef:      "some-run-image",
				}
				om.EXPECT().EnsureRegistryAccess(opts).Return(wasCalled)
				om.EXPECT().WithPrevious(opts.PreviousImageRef, opts.LaunchCacheDir).Return(wasCalled)
				om.EXPECT().WithRun(opts.RunImageRef).Return(wasCalled)

				_, err := af.NewAnalyzer(opts, logger)
				h.AssertNil(t, err)
				h.AssertEq(t, callCount, 3)
			})
		})

		when("platform api < 0.7", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "")
			})

			it("calls the expected operations", func() {
				opts := platform.AnalyzerOpts{
					LayersDir:        "some-layers-dir",
					LegacyCacheDir:   "some-cache-dir",
					LegacyGroupPath:  "some-group.toml",
					PreviousImageRef: "some-previous-image",
					RunImageRef:      "some-ignored-run-image",
				}
				om.EXPECT().WithBuildpacks(opts.LegacyGroup, opts.LegacyGroupPath).Return(wasCalled)
				om.EXPECT().WithCache(opts.CacheImageRef, opts.LegacyCacheDir).Return(wasCalled)
				om.EXPECT().WithLayerMetadataRestorer(opts.LayersDir, opts.SkipLayers, logger).Return(wasCalled)
				om.EXPECT().WithPrevious(opts.PreviousImageRef, opts.LaunchCacheDir).Return(wasCalled)

				_, err := af.NewAnalyzer(opts, logger)
				h.AssertNil(t, err)
				h.AssertEq(t, callCount, 4)
			})
		})
	}
}
