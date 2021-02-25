package lifecycle_test

import (
	"encoding/json"
	"io/ioutil"
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

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

func TestAnalyzer(t *testing.T) {
	spec.Run(t, "Analyzer", testAnalyzer, spec.Report(report.Terminal{}))
}

func testAnalyzer(t *testing.T, when spec.G, it spec.S) {
	var (
		analyzer          *lifecycle.Analyzer
		mockLayerAnalyzer *testmock.MockLayerAnalyzer
		mockCtrl          *gomock.Controller
		layerDir          string
		tmpDir            string
		cacheDir          string
		testCache         lifecycle.Cache
	)

	it.Before(func() {
		var err error

		tmpDir, err = ioutil.TempDir("", "analyzer-tests")
		h.AssertNil(t, err)

		layerDir, err = ioutil.TempDir("", "lifecycle-layer-dir")
		h.AssertNil(t, err)

		cacheDir, err = ioutil.TempDir("", "some-cache-dir")
		h.AssertNil(t, err)

		testCache, err = cache.NewVolumeCache(cacheDir)
		h.AssertNil(t, err)

		mockCtrl = gomock.NewController(t)
		mockLayerAnalyzer = testmock.NewMockLayerAnalyzer(mockCtrl)
		analyzer = &lifecycle.Analyzer{
			Buildpacks:    []buildpack.GroupBuildpack{{ID: "metadata.buildpack"}, {ID: "no.cache.buildpack"}, {ID: "no.metadata.buildpack"}},
			LayersDir:     layerDir,
			Logger:        &log.Logger{Handler: &discard.Handler{}},
			LayerAnalyzer: mockLayerAnalyzer,
			PlatformAPI:   api.Platform.Latest(),
		}
		if testing.Verbose() {
			analyzer.Logger = cmd.DefaultLogger
			h.AssertNil(t, cmd.SetLogLevel("debug"))
		}
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(tmpDir))
		h.AssertNil(t, os.RemoveAll(layerDir))
		h.AssertNil(t, os.RemoveAll(cacheDir))
		mockCtrl.Finish()
	})

	when("#Analyze", func() {
		var (
			appImageMetadata platform.LayersMetadata
			ref              *testmock.MockReference
			image            *fakes.Image
		)

		it.Before(func() {
			image = fakes.NewImage("image-repo-name", "", local.IDIdentifier{
				ImageID: "s0m3D1g3sT",
			})
			analyzer.Image = image
			analyzer.Cache = testCache
			ref = testmock.NewMockReference(mockCtrl)
			ref.EXPECT().Name().AnyTimes()
		})

		it.After(func() {
			h.AssertNil(t, image.Cleanup())
		})

		when("image exists", func() {
			it.Before(func() {
				metadata := h.MustReadFile(t, filepath.Join("testdata", "restorer", "app_metadata.json"))
				h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", string(metadata)))
				h.AssertNil(t, json.Unmarshal(metadata, &appImageMetadata))
			})

			it("returns the analyzed metadata", func() {
				md, err := analyzer.Analyze()
				h.AssertNil(t, err)

				h.AssertEq(t, md.Image.Reference, "s0m3D1g3sT")
				h.AssertEq(t, md.Metadata, appImageMetadata)
			})

			when("cache exists", func() {
				it.Before(func() {
					metadata := h.MustReadFile(t, filepath.Join("testdata", "restorer", "cache_metadata.json"))
					var cacheMetadata platform.CacheMetadata
					h.AssertNil(t, json.Unmarshal(metadata, &cacheMetadata))
					h.AssertNil(t, testCache.SetMetadata(cacheMetadata))
					h.AssertNil(t, testCache.Commit())

					analyzer.Buildpacks = append(analyzer.Buildpacks, buildpack.GroupBuildpack{ID: "escaped/buildpack/id"})
				})

				when("platform API < 0.6", func() {
					it.Before(func() {
						analyzer.PlatformAPI = api.MustParse("0.5")
					})

					it("analyzes layers", func() {
						mockLayerAnalyzer.EXPECT().Analyze(analyzer.Buildpacks, analyzer.SkipLayers, gomock.Any(), testCache)

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)
					})

					when("skip-layers is true", func() {
						it.Before(func() {
							analyzer.SkipLayers = true
							mockLayerAnalyzer.EXPECT().Analyze(analyzer.Buildpacks, analyzer.SkipLayers, gomock.Any(), testCache)
						})

						it("should return the analyzed metadata", func() {
							md, err := analyzer.Analyze()
							h.AssertNil(t, err)

							h.AssertEq(t, md.Image.Reference, "s0m3D1g3sT")
							h.AssertEq(t, md.Metadata, appImageMetadata)
						})
					})
				})
			})
		})

		when("platform API < 0.6", func() {
			it.Before(func() {
				analyzer.PlatformAPI = api.MustParse("0.5")
				analyzer.Cache = testCache
			})

			when("image is not found", func() {
				it.Before(func() {
					h.AssertNil(t, image.Delete())
				})

				when("cache exists", func() {
					it.Before(func() {
						metadata := h.MustReadFile(t, filepath.Join("testdata", "restorer", "cache_metadata.json"))
						var cacheMetadata platform.CacheMetadata
						h.AssertNil(t, json.Unmarshal(metadata, &cacheMetadata))
						h.AssertNil(t, testCache.SetMetadata(cacheMetadata))
						h.AssertNil(t, testCache.Commit())

						analyzer.Buildpacks = append(analyzer.Buildpacks, buildpack.GroupBuildpack{ID: "escaped/buildpack/id"})
						mockLayerAnalyzer.EXPECT().Analyze(analyzer.Buildpacks, analyzer.SkipLayers, gomock.Any(), testCache)
					})

					it("returns a nil image in the analyzed metadata", func() {
						md, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertNil(t, md.Image)
						h.AssertEq(t, md.Metadata, platform.LayersMetadata{})
					})
				})
				when("cache is empty", func() {
					it.Before(func() {
						mockLayerAnalyzer.EXPECT().Analyze(analyzer.Buildpacks, analyzer.SkipLayers, gomock.Any(), testCache)
					})
					it("does not restore any metadata", func() {
						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						files, err := ioutil.ReadDir(layerDir)
						h.AssertNil(t, err)
						h.AssertEq(t, len(files), 0)
					})
					it("returns a nil image in the analyzed metadata", func() {
						md, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertNil(t, md.Image)
						h.AssertEq(t, md.Metadata, platform.LayersMetadata{})
					})
				})
				when("cache is not provided", func() {
					it.Before(func() {
						testCache = nil
						analyzer.Cache = testCache
						mockLayerAnalyzer.EXPECT().Analyze(analyzer.Buildpacks, analyzer.SkipLayers, gomock.Any(), testCache)
					})
					it("does not restore any metadata", func() {
						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						files, err := ioutil.ReadDir(layerDir)
						h.AssertNil(t, err)
						h.AssertEq(t, len(files), 0)
					})
					it("returns a nil image in the analyzed metadata", func() {
						md, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertNil(t, md.Image)
						h.AssertEq(t, md.Metadata, platform.LayersMetadata{})
					})
				})
			})
		})

		when("image does not have metadata label", func() {
			it.Before(func() {
				h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", ""))
				analyzer.Cache = testCache
			})
			it("does not restore any metadata", func() {
				_, err := analyzer.Analyze()
				h.AssertNil(t, err)

				files, err := ioutil.ReadDir(layerDir)
				h.AssertNil(t, err)
				h.AssertEq(t, len(files), 0)
			})
			it("returns empty analyzed metadata", func() {
				md, err := analyzer.Analyze()
				h.AssertNil(t, err)
				h.AssertEq(t, md.Metadata, platform.LayersMetadata{})
			})
		})

		when("image has incompatible metadata", func() {
			it.Before(func() {
				h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", `{["bad", "metadata"]}`))
				analyzer.Cache = testCache
			})
			it("does not restore any metadata", func() {
				_, err := analyzer.Analyze()
				h.AssertNil(t, err)

				files, err := ioutil.ReadDir(layerDir)
				h.AssertNil(t, err)
				h.AssertEq(t, len(files), 0)
			})
			it("returns empty analyzed metadata", func() {
				md, err := analyzer.Analyze()
				h.AssertNil(t, err)
				h.AssertEq(t, md.Metadata, platform.LayersMetadata{})
			})
		})
	})
}
