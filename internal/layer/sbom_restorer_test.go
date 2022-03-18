package layer_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/platform"
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
		layersDir, err = ioutil.TempDir("", "lifecycle.layers-dir.")
		h.AssertNil(t, err)

		sbomRestorer = layer.NewSBOMRestorer(layersDir, &log.Logger{Handler: &discard.Handler{}})
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(layersDir))
	})

	when("#RestoreFromPrevious", func() {
		var (
			artifactsDir string
			layerDigest  string
			image        *fakes.Image
		)

		it.Before(func() {
			artifactsDir, err = ioutil.TempDir("", "lifecycle.artifacts-dir.")
			h.AssertNil(t, err)
			h.Mkdir(t, filepath.Join(layersDir, "sbom", "launch"))
			h.Mkfile(t, "some-bom-data", filepath.Join(layersDir, "sbom", "launch", "some-file"))
			factory := &layers.Factory{ArtifactsDir: artifactsDir}
			layer, err := factory.DirLayer("launch.sbom", filepath.Join(layersDir, "sbom", "launch"))
			h.AssertNil(t, err)
			layerDigest = layer.Digest

			image = fakes.NewImage("image-repo-name", "", local.IDIdentifier{
				ImageID: "s0m3D1g3sT",
			})
			h.AssertNil(t, image.AddLayerWithDiffID(layer.TarPath, layer.Digest))
			h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", fmt.Sprintf(`{"sbom": {"sha":"%s"}}`, layer.Digest)))

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
	})

	when("#RestoreFromCache", func() {
		var (
			artifactsDir string
			cacheDir     string
			layerDigest  string
			testCache    lifecycle.Cache
		)

		it.Before(func() {
			artifactsDir, err = ioutil.TempDir("", "lifecycle.artifacts-dir.")
			h.AssertNil(t, err)
			h.Mkdir(t, filepath.Join(layersDir, "sbom", "cache"))
			h.Mkfile(t, "some-bom-data", filepath.Join(layersDir, "sbom", "cache", "some-file"))
			factory := &layers.Factory{ArtifactsDir: artifactsDir}
			layer, err := factory.DirLayer("cache.sbom", filepath.Join(layersDir, "sbom", "cache"))
			h.AssertNil(t, err)
			layerDigest = layer.Digest

			cacheDir, err = ioutil.TempDir("", "lifecycle.cache-dir.")
			h.AssertNil(t, err)
			testCache, err = cache.NewVolumeCache(cacheDir)
			h.AssertNil(t, err)
			h.AssertNil(t, testCache.AddLayerFile(layer.TarPath, layer.Digest))
			h.AssertNil(t, testCache.SetMetadata(platform.CacheMetadata{BOM: platform.LayerMetadata{SHA: layer.Digest}}))
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
		var detectedBps []buildpack.GroupBuildpack

		it.Before(func() {
			h.Mkdir(t, filepath.Join(layersDir, "sbom"))
			h.RecursiveCopy(t,
				filepath.Join("testdata", "sbom"),
				filepath.Join(layersDir, "sbom"))

			buildpackAPI := api.Buildpack.Latest().String()
			detectedBps = []buildpack.GroupBuildpack{
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
