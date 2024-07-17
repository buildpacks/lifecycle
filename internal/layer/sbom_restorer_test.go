package layer_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestSBOMRestorer(t *testing.T) {
	spec.Run(t, "SBOMRestorer", testSBOMRestorer, spec.Report(report.Terminal{}))
}

func testSBOMRestorer(t *testing.T, when spec.G, it spec.S) {
	var (
		layersDir    string
		sbomRestorer layer.SBOMRestorer
		err          error
	)

	it.Before(func() {
		layersDir, err = os.MkdirTemp("", "lifecycle.layers-dir.")
		h.AssertNil(t, err)

		sbomRestorer = layer.NewSBOMRestorer(layer.SBOMRestorerOpts{
			LayersDir: layersDir,
			Logger:    &log.Logger{Handler: &discard.Handler{}},
		}, api.Platform.Latest())
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(layersDir))
	})

	when("#NewSBOMRestorer", func() {
		when("nop option is provided", func() {
			it("returns a NopSBOMRestorer", func() {
				r := layer.NewSBOMRestorer(layer.SBOMRestorerOpts{
					Nop: true,
				}, api.Platform.Latest())
				_, ok := r.(*layer.NopSBOMRestorer)
				h.AssertEq(t, ok, true)
			})
		})
		when("not supported by the platform", func() {
			it("returns a NopSBOMRestorer", func() {
				r := layer.NewSBOMRestorer(layer.SBOMRestorerOpts{
					Nop: true,
				}, api.MustParse("0.7"))
				_, ok := r.(*layer.NopSBOMRestorer)
				h.AssertEq(t, ok, true)
			})
		})
		when("nop option is not provided", func() {
			it("returns a DefaultSBOMRestorer", func() {
				r := layer.NewSBOMRestorer(layer.SBOMRestorerOpts{
					LayersDir: "some-dir",
					Logger:    &log.Logger{Handler: &discard.Handler{}},
				}, api.Platform.Latest())
				_, ok := r.(*layer.DefaultSBOMRestorer)
				h.AssertEq(t, ok, true)
			})
		})
	})

	when("#RestoreFromPrevious", func() {
		var (
			artifactsDir string
			layerDigest  string
			image        *fakes.Image
		)

		it.Before(func() {
			artifactsDir, err = os.MkdirTemp("", "lifecycle.artifacts-dir.")
			h.AssertNil(t, err)
			h.Mkdir(t, filepath.Join(layersDir, "sbom", "launch"))
			h.Mkfile(t, "some-bom-data", filepath.Join(layersDir, "sbom", "launch", "some-file"))
			factory := &layers.Factory{ArtifactsDir: artifactsDir}
			layer, err := factory.DirLayer("launch.sbom", filepath.Join(layersDir, "sbom", "launch"), "")
			h.AssertNil(t, err)
			layerDigest = layer.Digest

			image = fakes.NewImage("image-repo-name", "", local.IDIdentifier{
				ImageID: "s0m3D1g3sT",
			})
			h.AssertNil(t, image.AddLayerWithDiffID(layer.TarPath, layer.Digest))

			h.AssertNil(t, os.RemoveAll(filepath.Join(layersDir, "sbom")))
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(artifactsDir))
		})

		it("restores the SBOM layer from the previous image", func() {
			h.AssertNil(t, sbomRestorer.RestoreFromPrevious(image, layerDigest))

			got := h.MustReadFile(t, filepath.Join(layersDir, "sbom", "launch", "some-file"))
			want := `some-bom-data`
			h.AssertEq(t, string(got), want)
		})

		when("image is empty", func() {
			it("errors", func() {
				h.AssertError(t,
					sbomRestorer.RestoreFromPrevious(nil, layerDigest),
					fmt.Sprintf("restoring layer: previous image not found for \"%s\"", layerDigest),
				)
			})
		})

		when("image is not found", func() {
			it.Before(func() {
				h.AssertNil(t, image.Delete())
			})

			it("does not error", func() {
				h.AssertNil(t, sbomRestorer.RestoreFromPrevious(image, "s0m3D1g3sT"))
			})
		})

		when("layer digest is empty", func() {
			it("does not error", func() {
				h.AssertNil(t, sbomRestorer.RestoreFromPrevious(image, ""))
			})
		})
	})

	when("#RestoreFromCache", func() {
		var (
			artifactsDir string
			cacheDir     string
			layerDigest  string
			testCache    phase.Cache
		)

		it.Before(func() {
			artifactsDir, err = os.MkdirTemp("", "lifecycle.artifacts-dir.")
			h.AssertNil(t, err)
			h.Mkdir(t, filepath.Join(layersDir, "sbom", "cache"))
			h.Mkfile(t, "some-bom-data", filepath.Join(layersDir, "sbom", "cache", "some-file"))
			factory := &layers.Factory{ArtifactsDir: artifactsDir}
			layer, err := factory.DirLayer("cache.sbom", filepath.Join(layersDir, "sbom", "cache"), "")
			h.AssertNil(t, err)
			layerDigest = layer.Digest

			cacheDir, err = os.MkdirTemp("", "lifecycle.cache-dir.")
			h.AssertNil(t, err)
			testCache, err = cache.NewVolumeCache(cacheDir, &log.Logger{Handler: &discard.Handler{}})
			h.AssertNil(t, err)
			h.AssertNil(t, testCache.AddLayerFile(layer.TarPath, layer.Digest))
			h.AssertNil(t, testCache.SetMetadata(platform.CacheMetadata{BOM: files.LayerMetadata{SHA: layer.Digest}}))
			h.AssertNil(t, testCache.Commit())

			h.AssertNil(t, os.RemoveAll(filepath.Join(layersDir, "sbom")))
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(artifactsDir))
			h.AssertNil(t, os.RemoveAll(cacheDir))
		})

		it("restores the SBOM layer from the cache", func() {
			h.AssertNil(t, sbomRestorer.RestoreFromCache(testCache, layerDigest))

			got := h.MustReadFile(t, filepath.Join(layersDir, "sbom", "cache", "some-file"))
			want := `some-bom-data`
			h.AssertEq(t, string(got), want)
		})
	})

	when("#RestoreToBuildpackLayers", func() {
		var detectedBps []buildpack.GroupElement

		it.Before(func() {
			h.Mkdir(t, filepath.Join(layersDir, "sbom"))
			h.RecursiveCopy(t,
				filepath.Join("testdata", "sbom"),
				filepath.Join(layersDir, "sbom"))

			buildpackAPI := api.Buildpack.Latest().String()
			detectedBps = []buildpack.GroupElement{
				{ID: "buildpack.id", API: buildpackAPI},
				{ID: "escaped/buildpack/id", API: buildpackAPI},
			}
			for _, bp := range detectedBps {
				h.AssertNil(t, os.MkdirAll(filepath.Join(layersDir, launch.EscapeID(bp.ID)), 0755))
			}
		})

		it("copies the SBOM files for detected buildpacks and removes the layers/sbom directory", func() {
			h.AssertNil(t, sbomRestorer.RestoreToBuildpackLayers(detectedBps))

			got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-true.sbom.cdx.json"))
			want := `{"key": "some-cache-bom-content"}`
			h.AssertEq(t, string(got), want)

			got = h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "launch-true.sbom.cdx.json"))
			want = `{"key": "some-launch-bom-content"}`
			h.AssertEq(t, string(got), want)

			got = h.MustReadFile(t, filepath.Join(layersDir, "escaped_buildpack_id", "launch-true.sbom.cdx.json"))
			want = `{"key": "some-escaped-launch-bom-content"}`
			h.AssertEq(t, string(got), want)

			h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "undetected-buildpack.id", "launch-true.sbom.cdx.json"))
			h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "sbom"))
		})

		it("removes the layers/sbom directory", func() {
			h.AssertNil(t, sbomRestorer.RestoreToBuildpackLayers(detectedBps))
			h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "sbom"))
		})

		it("does not copy launch-level SBOM files", func() {
			h.AssertNil(t, sbomRestorer.RestoreToBuildpackLayers(detectedBps))
			h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "launch.sbom.cdx.json"))
		})

		when("the bp layers directory doesn't exist", func() {
			it.Before(func() {
				os.RemoveAll(filepath.Join(layersDir, "buildpack.id"))
				os.RemoveAll(filepath.Join(layersDir, "escaped_buildpack_id"))
			})

			it("does not error", func() {
				h.AssertNil(t, sbomRestorer.RestoreToBuildpackLayers(detectedBps))
			})
		})
	})
}
