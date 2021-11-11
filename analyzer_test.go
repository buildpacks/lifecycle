package lifecycle_test

import (
	"encoding/json"
	"fmt"
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
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

func TestAnalyzer(t *testing.T) {
	for _, api := range api.Platform.Supported {
		spec.Run(t, "unit-analyzer/"+api.String(), testAnalyzerBuilder(api.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testAnalyzerBuilder(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var (
			cacheDir         string
			layersDir        string
			tmpDir           string
			analyzer         *lifecycle.Analyzer
			image            *fakes.Image
			metadataRestorer *testmock.MockLayerMetadataRestorer
			mockCtrl         *gomock.Controller
			testCache        lifecycle.Cache
		)

		it.Before(func() {
			var err error

			tmpDir, err = ioutil.TempDir("", "analyzer-tests")
			h.AssertNil(t, err)

			layersDir, err = ioutil.TempDir("", "lifecycle-layer-dir")
			h.AssertNil(t, err)

			cacheDir, err = ioutil.TempDir("", "some-cache-dir")
			h.AssertNil(t, err)

			testCache, err = cache.NewVolumeCache(cacheDir)
			h.AssertNil(t, err)

			image = fakes.NewImage("image-repo-name", "", local.IDIdentifier{
				ImageID: "s0m3D1g3sT",
			})

			discardLogger := log.Logger{Handler: &discard.Handler{}}

			mockCtrl = gomock.NewController(t)
			metadataRestorer = testmock.NewMockLayerMetadataRestorer(mockCtrl)

			p, err := platform.NewPlatform(platformAPI)
			h.AssertNil(t, err)
			analyzer = &lifecycle.Analyzer{
				PreviousImage: image,
				Logger:        &discardLogger,
				Platform:      p,
				Buildpacks: []buildpack.GroupBuildpack{
					{ID: "metadata.buildpack", API: api.Buildpack.Latest().String()},
					{ID: "no.cache.buildpack", API: api.Buildpack.Latest().String()},
					{ID: "no.metadata.buildpack", API: api.Buildpack.Latest().String()},
				},
				Cache:                 testCache,
				LayerMetadataRestorer: metadataRestorer,
			}

			if testing.Verbose() {
				analyzer.Logger = cmd.DefaultLogger
				h.AssertNil(t, cmd.SetLogLevel("debug"))
			}
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tmpDir))
			h.AssertNil(t, os.RemoveAll(layersDir))
			h.AssertNil(t, os.RemoveAll(cacheDir))
			h.AssertNil(t, image.Cleanup())
			mockCtrl.Finish()
		})

		when("#Analyze", func() {
			var (
				expectedAppMetadata   platform.LayersMetadata
				expectedCacheMetadata platform.CacheMetadata
				ref                   *testmock.MockReference
			)

			expectRestoresLayerMetadataIfSupported := func() {
				if api.MustParse(analyzer.Platform.API()).LessThan("0.7") {
					useShaFiles := true
					layerSHAStore := lifecycle.NewLayerSHAStore(useShaFiles)
					metadataRestorer.EXPECT().Restore(analyzer.Buildpacks, expectedAppMetadata, expectedCacheMetadata, layerSHAStore)
				}
			}

			it.Before(func() {
				ref = testmock.NewMockReference(mockCtrl)
				ref.EXPECT().Name().AnyTimes()
			})

			when("image exists", func() {
				it.Before(func() {
					metadata := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "app_metadata.json"))
					h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", string(metadata)))
					h.AssertNil(t, json.Unmarshal(metadata, &expectedAppMetadata))
				})

				it("returns the analyzed metadata", func() {
					expectRestoresLayerMetadataIfSupported()

					md, err := analyzer.Analyze()
					h.AssertNil(t, err)

					h.AssertEq(t, md.PreviousImage.Reference, "s0m3D1g3sT")
					h.AssertEq(t, md.Metadata, expectedAppMetadata)
				})

				when("when there is BOM information", func() {
					var artifactsDir string

					it.Before(func() {
						h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.8"), "Platform API < 0.8 does not restore sBOM")

						var err error
						artifactsDir, err = ioutil.TempDir("", "lifecycle.artifacts-dir.")
						h.AssertNil(t, err)

						h.Mkdir(t, filepath.Join(layersDir, "sbom", "launch"))
						h.Mkfile(t, "some-bom-data", filepath.Join(layersDir, "sbom", "launch", "some-file"))

						factory := &layers.Factory{ArtifactsDir: artifactsDir}
						layer, err := factory.DirLayer("launch.bom", filepath.Join(layersDir, "sbom", "launch"))
						h.AssertNil(t, err)
						h.AssertNil(t, image.AddLayerWithDiffID(layer.TarPath, layer.Digest))
						h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", fmt.Sprintf(`{"sbom": {"sha":"%s"}}`, layer.Digest)))

						h.AssertNil(t, os.RemoveAll(filepath.Join(layersDir, "sbom")))
					})

					it.After(func() {
						h.AssertNil(t, os.RemoveAll(artifactsDir))
					})

					it("restores any BOM layers", func() {
						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						got := h.MustReadFile(t, filepath.Join(layersDir, "sbom", "launch", "some-file"))
						want := `some-bom-data`
						h.AssertEq(t, string(got), want)
					})
				})

				when("cache exists", func() {
					it.Before(func() {
						metadata := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "cache_metadata.json"))
						h.AssertNil(t, json.Unmarshal(metadata, &expectedCacheMetadata))
						h.AssertNil(t, testCache.SetMetadata(expectedCacheMetadata))
						h.AssertNil(t, testCache.Commit())

						analyzer.Buildpacks = append(analyzer.Buildpacks, buildpack.GroupBuildpack{ID: "escaped/buildpack/id", API: api.Buildpack.Latest().String()})
						expectRestoresLayerMetadataIfSupported()
					})

					it("returns the analyzed metadata", func() {
						md, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertEq(t, md.Metadata, expectedAppMetadata)
					})
				})

				when("cache exists with BOM information", func() {
					var artifactsDir string

					it.Before(func() {
						h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.8"), "Platform API < 0.8 does not restore sBOM")

						var err error
						artifactsDir, err = ioutil.TempDir("", "lifecycle.artifacts-dir.")
						h.AssertNil(t, err)

						h.Mkdir(t, filepath.Join(layersDir, "sbom", "cache"))
						h.Mkfile(t, "some-bom-data", filepath.Join(layersDir, "sbom", "cache", "some-file"))

						factory := &layers.Factory{ArtifactsDir: artifactsDir}
						layer, err := factory.DirLayer("cache.bom", filepath.Join(layersDir, "sbom", "cache"))
						h.AssertNil(t, err)
						h.AssertNil(t, testCache.AddLayerFile(layer.TarPath, layer.Digest))
						h.AssertNil(t, testCache.SetMetadata(platform.CacheMetadata{BOM: platform.LayerMetadata{SHA: layer.Digest}}))
						h.AssertNil(t, testCache.Commit())

						h.AssertNil(t, os.RemoveAll(filepath.Join(layersDir, "sbom")))
					})

					it.After(func() {
						h.AssertNil(t, os.RemoveAll(artifactsDir))
					})

					it("restores any BOM layers", func() {
						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						got := h.MustReadFile(t, filepath.Join(layersDir, "sbom", "cache", "some-file"))
						want := `some-bom-data`
						h.AssertEq(t, string(got), want)
					})
				})
			})

			when("image not found", func() {
				it.Before(func() {
					h.AssertNil(t, image.Delete())
					expectRestoresLayerMetadataIfSupported()
				})

				it("returns a nil image in the analyzed metadata", func() {
					md, err := analyzer.Analyze()
					h.AssertNil(t, err)

					h.AssertNil(t, md.PreviousImage)
					h.AssertEq(t, md.Metadata, platform.LayersMetadata{})
				})
			})

			when("image does not have metadata label", func() {
				it.Before(func() {
					h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", ""))
					expectRestoresLayerMetadataIfSupported()
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
					expectRestoresLayerMetadataIfSupported()
				})

				it("returns empty analyzed metadata", func() {
					md, err := analyzer.Analyze()
					h.AssertNil(t, err)
					h.AssertEq(t, md.Metadata, platform.LayersMetadata{})
				})
			})

			when("run image is provided", func() {
				it.Before(func() {
					analyzer.RunImage = image
				})

				it("returns the run image digest in the analyzed metadata", func() {
					expectRestoresLayerMetadataIfSupported()

					md, err := analyzer.Analyze()
					h.AssertNil(t, err)

					h.AssertEq(t, md.RunImage.Reference, "s0m3D1g3sT")
				})
			})
		})
	}
}
