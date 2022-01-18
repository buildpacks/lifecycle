package layout_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/types"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/layout"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestImage(t *testing.T) {
	spec.Run(t, "LayoutImage", testImage, spec.Report(report.Terminal{}))
}

func testImage(t *testing.T, when spec.G, it spec.S) {
	var tmpDir string

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "layout")
		h.AssertNil(t, err)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#Found", func() {
		when("directory doesn't exist", func() {
			it("returns false", func() {
				img, err := layout.NewImage(filepath.Join(tmpDir, "non-existent"))
				h.AssertNil(t, err)
				h.AssertEq(t, img.Found(), false)
			})
		})

		when("directory exists", func() {
			it.Before(func() {
				err := os.MkdirAll(filepath.Join(tmpDir, "some-image"), os.ModePerm)
				h.AssertNil(t, err)
			})

			it("returns true", func() {
				img, err := layout.NewImage(filepath.Join(tmpDir, "some-image"))
				h.AssertNil(t, err)
				h.AssertEq(t, img.Found(), true)
			})
		})
	})

	when("#Save", func() {
		when("no base image is provided", func() {
			it("creates layout", func() {
				layoutDir := filepath.Join(tmpDir, "layout")

				// create image
				image, err := layout.NewImage(layoutDir)
				h.AssertNil(t, err)

				// add a random layer
				path, diffID, _ := h.RandomLayer(t, tmpDir)
				err = image.AddLayerWithDiffID(path, diffID)
				h.AssertNil(t, err)

				// save
				err = image.Save()
				h.AssertNil(t, err)

				// check minimal
				indexPath := filepath.Join(layoutDir, "index.json")
				h.AssertPathExists(t, filepath.Join(layoutDir, "oci-layout"))
				h.AssertPathExists(t, indexPath)

				// check index file
				data, err := ioutil.ReadFile(indexPath)
				h.AssertNil(t, err)

				t.Log("INDEX: ", string(data))

				index := &v1.IndexManifest{}
				err = json.Unmarshal(data, index)
				h.AssertNil(t, err)

				// check manifest file
				h.AssertEq(t, len(index.Manifests), 1)
				digest := index.Manifests[0].Digest
				manifestPath := filepath.Join(layoutDir, "blobs", digest.Algorithm, digest.Hex)
				h.AssertPathExists(t, manifestPath)

				data, err = ioutil.ReadFile(manifestPath)
				h.AssertNil(t, err)

				t.Log("MANIFEST: ", string(data))

				manifest := &v1.Manifest{}
				err = json.Unmarshal(data, manifest)
				h.AssertNil(t, err)

				mediaType := manifest.Config.MediaType
				h.AssertEq(t, mediaType, types.OCIConfigJSON)

				// check config file
				digest = manifest.Config.Digest
				configPath := filepath.Join(layoutDir, "blobs", digest.Algorithm, digest.Hex)
				h.AssertPathExists(t, configPath)

				data, err = ioutil.ReadFile(configPath)
				h.AssertNil(t, err)

				t.Log("CONFIG: ", string(data))

				configFile := &v1.ConfigFile{}
				err = json.Unmarshal(data, configFile)
				h.AssertNil(t, err)

				// check layer
				h.AssertEq(t, len(configFile.RootFS.DiffIDs), 1)
				h.AssertEq(t, len(manifest.Layers), 1)

				digest = manifest.Layers[0].Digest
				layerPath := filepath.Join(layoutDir, "blobs", digest.Algorithm, digest.Hex)
				h.AssertPathExists(t, layerPath)

				// TODO: Check that layer is not compressed
				mediaType = manifest.Layers[0].MediaType
				h.AssertEq(t, mediaType, types.OCILayer)
			})
		})

		when("base image is provided", func() {
			var (
				baseImage *layout.Image
			)

			it.Before(func() {
				baseImageDir := filepath.Join(tmpDir, "base-image")

				// create image
				var err error
				baseImage, err = layout.NewImage(baseImageDir)
				h.AssertNil(t, err)

				// add a random layer
				path, diffID, _ := h.RandomLayer(t, tmpDir)
				err = baseImage.AddLayerWithDiffID(path, diffID)
				h.AssertNil(t, err)

				// save
				err = baseImage.Save()
				h.AssertNil(t, err)
			})

			it("creates layout", func() {
				layoutDir := filepath.Join(tmpDir, "layout")

				// create image
				image, err := layout.NewImage(layoutDir, layout.FromBaseImage(baseImage.Name()))
				h.AssertNil(t, err)

				// add a random layer
				path, diffID, _ := h.RandomLayer(t, tmpDir)
				err = image.AddLayerWithDiffID(path, diffID)
				h.AssertNil(t, err)

				// save
				err = image.Save()
				h.AssertNil(t, err)

				top, err := image.TopLayer()
				h.AssertNil(t, err)
				t.Logf("TOP LAYER %s", top)

				// check minimal
				indexPath := filepath.Join(layoutDir, "index.json")
				h.AssertPathExists(t, filepath.Join(layoutDir, "oci-layout"))
				h.AssertPathExists(t, indexPath)

				// check index file
				data, err := ioutil.ReadFile(indexPath)
				h.AssertNil(t, err)

				t.Log("INDEX: ", string(data))

				index := &v1.IndexManifest{}
				err = json.Unmarshal(data, index)
				h.AssertNil(t, err)

				// check manifest file
				h.AssertEq(t, len(index.Manifests), 1)
				digest := index.Manifests[0].Digest
				manifestPath := filepath.Join(layoutDir, "blobs", digest.Algorithm, digest.Hex)
				h.AssertPathExists(t, manifestPath)

				data, err = ioutil.ReadFile(manifestPath)
				h.AssertNil(t, err)

				t.Log("MANIFEST: ", string(data))

				manifest := &v1.Manifest{}
				err = json.Unmarshal(data, manifest)
				h.AssertNil(t, err)

				// check config file
				digest = manifest.Config.Digest
				configPath := filepath.Join(layoutDir, "blobs", digest.Algorithm, digest.Hex)
				h.AssertPathExists(t, configPath)

				data, err = ioutil.ReadFile(configPath)
				h.AssertNil(t, err)

				t.Log("CONFIG: ", string(data))

				configFile := &v1.ConfigFile{}
				err = json.Unmarshal(data, configFile)
				h.AssertNil(t, err)

				// check layers
				h.AssertEq(t, len(configFile.RootFS.DiffIDs), 2)
				h.AssertEq(t, len(manifest.Layers), 2)

				digest = manifest.Layers[0].Digest
				layerPath := filepath.Join(layoutDir, "blobs", digest.Algorithm, digest.Hex)
				h.AssertPathExists(t, layerPath)

				mediaType := manifest.Layers[0].MediaType
				h.AssertEq(t, mediaType, types.OCILayer)

				digest = manifest.Layers[1].Digest
				layerPath = filepath.Join(layoutDir, "blobs", digest.Algorithm, digest.Hex)
				h.AssertPathExists(t, layerPath)

				mediaType = manifest.Layers[1].MediaType
				h.AssertEq(t, mediaType, types.OCILayer)
				// TODO: Check that layer is not compressed (types.OCIUncompressedLayer)
			})
		})
	})
}
