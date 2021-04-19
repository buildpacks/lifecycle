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

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

func testAnalyzer07(t *testing.T, when spec.G, it spec.S) {
	var (
		analyzer   *lifecycle.Analyzer
		mockCtrl   *gomock.Controller
		layerDir   string
		tmpDir     string
		cacheDir   string
		skipLayers bool
	)

	it.Before(func() {
		var err error

		tmpDir, err = ioutil.TempDir("", "analyzer-tests")
		h.AssertNil(t, err)

		layerDir, err = ioutil.TempDir("", "lifecycle-layer-dir")
		h.AssertNil(t, err)

		cacheDir, err = ioutil.TempDir("", "some-cache-dir")
		h.AssertNil(t, err)

		platform := platform.NewPlatform("0.7")

		discardLogger := log.Logger{Handler: &discard.Handler{}}
		analyzer = &lifecycle.Analyzer{
			Buildpacks: []buildpack.GroupBuildpack{
				{ID: "metadata.buildpack", API: api.Buildpack.Latest().String()},
				{ID: "no.cache.buildpack", API: api.Buildpack.Latest().String()},
				{ID: "no.metadata.buildpack", API: api.Buildpack.Latest().String()},
			},
			Logger: &discardLogger,
			LayerMetadataRestorer: lifecycle.NewLayerMetadataRestorer(
				&discardLogger,
				&lifecycle.DefaultCacheMetadataRetriever{
					Logger: &discardLogger,
				},
				layerDir,
				platform,
				skipLayers),
			Platform: platform,
		}
		if testing.Verbose() {
			analyzer.Logger = cmd.DefaultLogger
			h.AssertNil(t, cmd.SetLogLevel("debug"))
		}
		mockCtrl = gomock.NewController(t)
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(tmpDir))
		h.AssertNil(t, os.RemoveAll(layerDir))
		h.AssertNil(t, os.RemoveAll(cacheDir))
		mockCtrl.Finish()
	})

	when("#Analyze", func() {
		var (
			image            *fakes.Image
			appImageMetadata platform.LayersMetadata
			ref              *testmock.MockReference
		)

		it.Before(func() {
			image = fakes.NewImage("image-repo-name", "", local.IDIdentifier{
				ImageID: "s0m3D1g3sT",
			})
			analyzer.Image = image
			analyzer.Cache = nil
			ref = testmock.NewMockReference(mockCtrl)
			ref.EXPECT().Name().AnyTimes()
		})

		it.After(func() {
			h.AssertNil(t, image.Cleanup())
		})

		when("image exists", func() {
			it.Before(func() {
				metadata := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "app_metadata.json"))
				h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", string(metadata)))
				h.AssertNil(t, json.Unmarshal(metadata, &appImageMetadata))
			})

			it("returns the analyzed metadata", func() {
				md, err := analyzer.Analyze()
				h.AssertNil(t, err)

				h.AssertEq(t, md.Image.Reference, "s0m3D1g3sT")
				h.AssertEq(t, md.Metadata, appImageMetadata)
			})

			when("image does not have metadata label", func() {
				it.Before(func() {
					h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", ""))
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

		when("image is not found", func() {
			it.Before(func() {
				h.AssertNil(t, image.Delete())
			})
			it("does not return layer metadata", func() {
				r, err := analyzer.Analyze()
				h.AssertNil(t, err)

				h.AssertEq(t, r.Metadata, platform.LayersMetadata{})
			})

			it("does not return an image identifier", func() {
				r, err := analyzer.Analyze()
				h.AssertNil(t, err)

				h.AssertNil(t, r.Image)
			})
		})
	})
}
