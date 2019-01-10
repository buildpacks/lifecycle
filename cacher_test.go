package lifecycle_test

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestCacher(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Cacher", testCacher, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCacher(t *testing.T, when spec.G, it spec.S) {
	var (
		cacher         lifecycle.Cacher
		metadata       lifecycle.AppImageMetadata
		uid            = 1234
		gid            = 4321
		fakeCacheImage *h.FakeImage
	)

	it.Before(func() {
		fakeCacheImage = h.NewFakeImage(t, "cacheImageName", "some-top-layer-sha", "some-cache-image-digest")
	})

	when("LocalImageCacher", func() {
		when("has one cache and launch layer", func() {
			it.Before(func() {
				cacher = &lifecycle.LocalImageCacher{
					BaseImage: fakeCacheImage,
					RepoName:  "myapp-123",
					Out:       log.New(os.Stdout, "", log.LstdFlags),
				}

				metadataJSON, err := ioutil.ReadFile(filepath.Join("testdata", "cacher", "metadata.json"))
				h.AssertNil(t, err)

				if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}

				layerMetadata := metadata.Buildpacks[0].Layers["layer2"]
				layerMetadata.Tar = filepath.Join("testdata", "cacher", "layer2.tar")
				layerMetadata.Launch = true
				metadata.Buildpacks[0].Layers["layer2"] = layerMetadata
			})

			when("#Export", func() {

				it("creates a layer on cache image", func() {
					layerDir := filepath.Join("./workspace", "buildpack.id", "layer2")

					h.AssertNil(t, cacher.Export(&metadata))
					h.AssertEq(t, fakeCacheImage.NumberOfLayers(), 1)
					h.AssertEq(t, fakeCacheImage.IsSaved(), true)

					layerTarPath := fakeCacheImage.FindLayerWithPath(layerDir)

					assertTarFileContents(t, layerTarPath, filepath.Join(layerDir, "file-from-layer-2"), "echo text from layer 2\n")
					assertTarFileOwner(t, layerTarPath, layerDir, uid, gid)
				})

			})
		})

		when("has one cache-only layer", func() {
			it.Before(func() {
				cacher = &lifecycle.LocalImageCacher{
					BaseImage: fakeCacheImage,
					RepoName:  "myapp-123",
					Out:       log.New(os.Stdout, "", log.LstdFlags),
				}

				metadataJSON, err := ioutil.ReadFile(filepath.Join("testdata", "cacher", "metadata.json"))
				h.AssertNil(t, err)

				if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}

				layerMetadata := metadata.Buildpacks[0].Layers["layer2"]
				layerMetadata.Tar = filepath.Join("testdata", "cacher", "layer2.tar")
				layerMetadata.Launch = false
				metadata.Buildpacks[0].Layers["layer2"] = layerMetadata
			})

			when("#Export", func() {

				it("creates a layer on cache image", func() {
					layerDir := filepath.Join("./workspace", "buildpack.id", "layer2")

					h.AssertNil(t, cacher.Export(&metadata))
					h.AssertEq(t, fakeCacheImage.NumberOfLayers(), 1)
					h.AssertEq(t, fakeCacheImage.IsSaved(), true)

					layerTarPath := fakeCacheImage.FindLayerWithPath(layerDir)

					assertTarFileContents(t, layerTarPath, filepath.Join(layerDir, "file-from-layer-2"), "echo text from layer 2\n")
					assertTarFileOwner(t, layerTarPath, layerDir, uid, gid)
				})

			})
		})

		when("has no cache layers", func() {
			it.Before(func() {
				cacher = &lifecycle.LocalImageCacher{
					BaseImage: fakeCacheImage,
					RepoName:  "myapp-123",
					Out:       log.New(os.Stdout, "", log.LstdFlags),
				}

				metadataJSON, err := ioutil.ReadFile(filepath.Join("testdata", "cacher", "metadata.json"))
				h.AssertNil(t, err)

				if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
					t.Fatalf("badly formatted metadata: %s", err)
				}

				layerMetadata := metadata.Buildpacks[0].Layers["layer2"]
				layerMetadata.Tar = filepath.Join("testdata", "cacher", "layer2.tar")
				layerMetadata.Cache = false
				metadata.Buildpacks[0].Layers["layer2"] = layerMetadata
			})

			when("#Export", func() {
				it("creates cache image with no layers", func() {
					h.AssertNil(t, cacher.Export(&metadata))
					h.AssertEq(t, fakeCacheImage.NumberOfLayers(), 0)
					h.AssertEq(t, fakeCacheImage.IsSaved(), true)
				})
			})
		})
	})
}
