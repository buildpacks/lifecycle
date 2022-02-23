package platform_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/platform"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/platform/testmock"
	"github.com/buildpacks/lifecycle/internal/layer"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestAnalyzerFactory(t *testing.T) {
	for _, api := range api.Platform.Supported {
		spec.Run(t, "unit-analyzer-factory/"+api.String(), testAnalyzerFactory(api.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testAnalyzerFactory(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var (
			af                    *platform.AnalyzerFactory
			fakeImageHandler      *testmock.MockImageHandler
			fakeRegistryValidator *testmock.MockRegistryValidator
			logHandler            *memory.Handler
			logger                *log.Logger
			mockController        *gomock.Controller
			tempDir               string
		)
		it.Before(func() {
			mockController = gomock.NewController(t)
			fakeImageHandler = testmock.NewMockImageHandler(mockController)
			fakeRegistryValidator = testmock.NewMockRegistryValidator(mockController)
			af = &platform.AnalyzerFactory{
				PlatformAPI:       api.MustParse(platformAPI),
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

		// TODO: test logger

		when("latest platform api(s)", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.8"), "")
			})

			when("provided a group", func() {
				it.Before(func() {
					fakeRegistryValidator.EXPECT().ReadableRegistryImages().AnyTimes()  // ignore
					fakeRegistryValidator.EXPECT().WriteableRegistryImages().AnyTimes() // ignore
				})

				it("ignores it", func() {
					opts := platform.AnalyzerOpts{
						LegacyGroup: buildpack.Group{
							Group: []buildpack.GroupBuildpack{{ID: "some-buildpack-id"}},
						},
					}

					analyzer, err := af.NewAnalyzer(opts, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, len(analyzer.Buildpacks), 0)
				})
			})

			when("provided a cache image", func() {
				it("validates registry access", func() {
					// TODO
				})
			})

			when("provided a cache directory", func() {
				it.Before(func() {
					fakeRegistryValidator.EXPECT().ReadableRegistryImages().AnyTimes()  // ignore
					fakeRegistryValidator.EXPECT().WriteableRegistryImages().AnyTimes() // ignore
				})

				it("ignores it", func() {
					opts := platform.AnalyzerOpts{
						LegacyCacheDir: "some-cache-dir",
					}

					analyzer, err := af.NewAnalyzer(opts, logger)
					h.AssertNil(t, err)
					h.AssertNil(t, analyzer.Cache)
				})
			})

			when("previous image", func() {
				it.Before(func() {
					fakeRegistryValidator.EXPECT().ReadableRegistryImages().AnyTimes()  // ignore
					fakeRegistryValidator.EXPECT().WriteableRegistryImages().AnyTimes() // ignore
				})

				it("provides it to the analyzer", func() {
					opts := platform.AnalyzerOpts{
						PreviousImageRef: "some-previous-image-ref",
					}
					previousImage := fakes.NewImage(opts.PreviousImageRef, "", nil)
					fakeImageHandler.EXPECT().InitImage(opts.PreviousImageRef).Return(previousImage, nil)
					fakeImageHandler.EXPECT().Docker() // ignore

					analyzer, err := af.NewAnalyzer(opts, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, analyzer.PreviousImage.Name(), opts.PreviousImageRef)
				})

				when("daemon case", func() {
					when("provided a launch cache dir", func() {
						it("previous image is a caching image", func() {
							opts := platform.AnalyzerOpts{
								PreviousImageRef: "some-previous-image-ref",
								LaunchCacheDir:   filepath.Join(tempDir, "launch-cache"),
							}
							previousImage := fakes.NewImage(opts.PreviousImageRef, "", nil)
							fakeImageHandler.EXPECT().InitImage(opts.PreviousImageRef).Return(previousImage, nil)
							fakeImageHandler.EXPECT().Docker().Return(true)

							analyzer, err := af.NewAnalyzer(opts, logger)
							h.AssertNil(t, err)
							h.AssertEq(t, analyzer.PreviousImage.Name(), opts.PreviousImageRef)
							_, ok := analyzer.PreviousImage.(*cache.CachingImage)
							h.AssertEq(t, ok, true)
							h.AssertPathExists(t, filepath.Join(tempDir, "launch-cache", "committed"))
							h.AssertPathExists(t, filepath.Join(tempDir, "launch-cache", "staging"))
						})
					})
				})
			})

			when("run image", func() {
				it.Before(func() {
					fakeRegistryValidator.EXPECT().ReadableRegistryImages().AnyTimes()  // ignore
					fakeRegistryValidator.EXPECT().WriteableRegistryImages().AnyTimes() // ignore
				})

				it("provides it to the analyzer", func() {
					opts := platform.AnalyzerOpts{
						RunImageRef: "some-run-image-ref",
					}
					runImage := fakes.NewImage(opts.RunImageRef, "", nil)
					fakeImageHandler.EXPECT().InitImage(opts.RunImageRef).Return(runImage, nil)

					analyzer, err := af.NewAnalyzer(opts, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, analyzer.RunImage.Name(), opts.RunImageRef)
				})
			})

			it("does not restore layer metadata", func() {
				fakeRegistryValidator.EXPECT().ReadableRegistryImages().AnyTimes()  // ignore
				fakeRegistryValidator.EXPECT().WriteableRegistryImages().AnyTimes() // ignore

				opts := platform.AnalyzerOpts{
					LayersDir: "some-layers-dir",
				}

				analyzer, err := af.NewAnalyzer(opts, logger)
				h.AssertNil(t, err)
				h.AssertNil(t, analyzer.LayerMetadataRestorer)
			})

			it("restores sbom layers from the previous image", func() {
				fakeRegistryValidator.EXPECT().ReadableRegistryImages().AnyTimes()  // ignore
				fakeRegistryValidator.EXPECT().WriteableRegistryImages().AnyTimes() // ignore

				opts := platform.AnalyzerOpts{
					LayersDir: "some-layers-dir",
				}

				analyzer, err := af.NewAnalyzer(opts, logger)
				h.AssertNil(t, err)
				defaultRestorer, ok := analyzer.SBOMRestorer.(*layer.DefaultSBOMRestorer)
				h.AssertEq(t, ok, true)
				h.AssertEq(t, defaultRestorer.LayersDir, opts.LayersDir)
			})
		})

		when("platform api < 0.8", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.8"), "")
			})

			when("previous image", func() {
				it.Before(func() {
					fakeRegistryValidator.EXPECT().ReadableRegistryImages().AnyTimes()  // ignore
					fakeRegistryValidator.EXPECT().WriteableRegistryImages().AnyTimes() // ignore
				})

				it("provides it to the analyzer", func() {
					opts := platform.AnalyzerOpts{
						LegacyGroup: buildpack.Group{
							Group: []buildpack.GroupBuildpack{{ID: "some-buildpack-id"}},
						}, // ignore
						PreviousImageRef: "some-previous-image-ref",
					}
					previousImage := fakes.NewImage(opts.PreviousImageRef, "", nil)
					fakeImageHandler.EXPECT().InitImage(opts.PreviousImageRef).Return(previousImage, nil)
					fakeImageHandler.EXPECT().Docker() // ignore

					analyzer, err := af.NewAnalyzer(opts, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, analyzer.PreviousImage.Name(), opts.PreviousImageRef)
				})
			})

			it("does not restore sbom layers from the previous image", func() {
				fakeRegistryValidator.EXPECT().ReadableRegistryImages().AnyTimes()  // ignore
				fakeRegistryValidator.EXPECT().WriteableRegistryImages().AnyTimes() // ignore

				opts := platform.AnalyzerOpts{
					LayersDir: "some-layers-dir",
					LegacyGroup: buildpack.Group{
						Group: []buildpack.GroupBuildpack{{ID: "some-buildpack-id"}},
					}, // ignore
				}

				analyzer, err := af.NewAnalyzer(opts, logger)
				h.AssertNil(t, err)
				h.AssertNil(t, analyzer.SBOMRestorer)
			})
		})

		when("platform api < 0.7", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "")
			})

			when("provided a group", func() {
				it("reads group.toml", func() {
					opts := platform.AnalyzerOpts{
						LegacyGroupPath: filepath.Join("testdata", "layers", "group.toml"),
					}

					analyzer, err := af.NewAnalyzer(opts, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, analyzer.Buildpacks, []buildpack.GroupBuildpack{
						{ID: "some-buildpack-id", Version: "some-buildpack-version", API: "0.7", Homepage: "some-buildpack-homepage"},
					})
				})

				it.Pend("validates buildpack apis", func() {
					opts := platform.AnalyzerOpts{
						LegacyGroupPath: filepath.Join("testdata", "layers", "group.toml"),
					}

					_, err := af.NewAnalyzer(opts, logger)
					h.AssertNotNil(t, err)
				})
			})

			when("provided a cache image", func() {
				it("provides it to the analyzer", func() {
					// opts := platform.AnalyzerOpts{
					//	CacheImageRef: "some-cache-image",
					// }
					// cacheImage := fakes.NewImage(opts.CacheImageRef, "", nil)
					// TODO: make cache factory
				})
			})

			when("provided a cache directory", func() {
				it("provides it to the analyzer", func() {
					opts := platform.AnalyzerOpts{
						LegacyCacheDir: filepath.Join(tempDir, "cache"),
						LegacyGroup: buildpack.Group{
							Group: []buildpack.GroupBuildpack{{ID: "some-buildpack-id"}},
						}, // ignore
					}
					fakeImageHandler.EXPECT().Keychain() // ignore

					analyzer, err := af.NewAnalyzer(opts, logger)
					h.AssertNil(t, err)
					cacheDir, ok := analyzer.Cache.(*cache.VolumeCache)
					h.AssertEq(t, ok, true)
					h.AssertEq(t, cacheDir.Name(), filepath.Join(tempDir, "cache"))
				})
			})

			when("previous image", func() {
				it("provides it to the analyzer", func() {
					opts := platform.AnalyzerOpts{
						LegacyGroup: buildpack.Group{
							Group: []buildpack.GroupBuildpack{{ID: "some-buildpack-id"}},
						}, // ignore
						PreviousImageRef: "some-previous-image-ref",
					}
					previousImage := fakes.NewImage(opts.PreviousImageRef, "", nil)
					fakeImageHandler.EXPECT().InitImage(opts.PreviousImageRef).Return(previousImage, nil)
					fakeImageHandler.EXPECT().Docker() // ignore

					analyzer, err := af.NewAnalyzer(opts, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, analyzer.PreviousImage.Name(), opts.PreviousImageRef)
				})
			})

			when("provided a run image", func() {
				it("ignores it", func() {
					opts := platform.AnalyzerOpts{
						LegacyGroup: buildpack.Group{
							Group: []buildpack.GroupBuildpack{{ID: "some-buildpack-id"}},
						}, // ignore
						RunImageRef: "some-run-image",
					}

					analyzer, err := af.NewAnalyzer(opts, logger)
					h.AssertNil(t, err)
					h.AssertNil(t, analyzer.RunImage)
				})
			})

			it("restores layer metadata", func() {
				opts := platform.AnalyzerOpts{
					LayersDir: "some-layers-dir",
					LegacyGroup: buildpack.Group{
						Group: []buildpack.GroupBuildpack{{ID: "some-buildpack-id"}},
					}, // ignore
				}

				analyzer, err := af.NewAnalyzer(opts, logger)
				h.AssertNil(t, err)
				defaultRestorer, ok := analyzer.LayerMetadataRestorer.(*layer.DefaultMetadataRestorer)
				h.AssertEq(t, ok, true)
				h.AssertEq(t, defaultRestorer.LayersDir, opts.LayersDir)
			})
		})
	}
}
