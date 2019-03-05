package lifecycle_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	h "github.com/buildpack/lifecycle/testhelpers"
	"github.com/buildpack/lifecycle/testmock"
)

func TestCacher(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Cacher", testCacher, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCacher(t *testing.T, when spec.G, it spec.S) {
	when("#Cacher", func() {
		var (
			cacher                 *lifecycle.Cacher
			layersDir              string
			cacheTrueLayerSHA      string
			otherBuildpackLayerSHA string
			tmpDir                 string
		)

		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "lifecycle.cacher.layer")
			h.AssertNil(t, err)
			cacher = &lifecycle.Cacher{
				ArtifactsDir: tmpDir,
				Buildpacks: []*lifecycle.Buildpack{
					{ID: "buildpack.id"},
					{ID: "other.buildpack.id"},
				},
				Out: log.New(ioutil.Discard, "", 0),
				UID: 1234,
				GID: 4321,
			}
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})

		when("the layers are valid", func() {
			it.Before(func() {
				layersDir = filepath.Join("testdata", "cacher", "layers")
				cacheTrueLayerSHA = "sha256:" + h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "buildpack.id/cache-true-layer"), 1234, 4321)
				otherBuildpackLayerSHA = "sha256:" + h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "other.buildpack.id/other-buildpack-layer"), 1234, 4321)
			})

			when("there is no previous cached image", func() {
				var (
					mockNonExistingOriginalImage *testmock.MockImage
				)

				it.Before(func() {
					mockNonExistingOriginalImage = testmock.NewMockImage(gomock.NewController(t))
					mockNonExistingOriginalImage.EXPECT().Found().Return(false, nil)
					mockNonExistingOriginalImage.EXPECT().Label("io.buildpacks.lifecycle.cache.metadata").
						Return("", errors.New("not exist")).AnyTimes()
					mockNonExistingOriginalImage.EXPECT().Name().Return("existing-previous-cache-image").AnyTimes()
				})

				it("exports cached layers to an image", func() {
					cacheImage := h.NewFakeImage(t, "cache-image", "", "")
					err := cacher.Cache(layersDir, mockNonExistingOriginalImage, cacheImage)
					h.AssertNil(t, err)

					layerPath := cacheImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/cache-true-layer"))

					assertTarFileContents(t,
						layerPath,
						filepath.Join(layersDir, "buildpack.id/cache-true-layer/file-from-cache-true-layer"),
						"file-from-cache-true-contents")

					h.AssertEq(t, cacheImage.IsSaved(), true)
				})

				it("sets the uid and gid of the layer contents", func() {
					cacheImage := h.NewFakeImage(t, "cache-image", "", "")
					err := cacher.Cache(layersDir, mockNonExistingOriginalImage, cacheImage)
					h.AssertNil(t, err)

					layerPath := cacheImage.FindLayerWithPath(filepath.Join(layersDir, "buildpack.id/cache-true-layer"))

					assertTarFileOwner(t,
						layerPath,
						filepath.Join(layersDir, "buildpack.id/cache-true-layer/file-from-cache-true-layer"),
						1234, 4321)

					h.AssertEq(t, cacheImage.IsSaved(), true)
				})

				it("sets label metadata", func() {
					cacheImage := h.NewFakeImage(t, "cache-image", "", "")
					err := cacher.Cache(layersDir, mockNonExistingOriginalImage, cacheImage)
					h.AssertNil(t, err)

					metadataJSON, err := cacheImage.Label("io.buildpacks.lifecycle.cache.metadata")
					h.AssertNil(t, err)

					var metadata lifecycle.CacheImageMetadata
					if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
						t.Fatalf("badly formatted metadata: %s", err)
					}

					t.Log("adds layer shas to metadata label")
					h.AssertEq(t, metadata.Buildpacks[0].ID, "buildpack.id")
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].SHA, cacheTrueLayerSHA)
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Launch, true)
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Build, false)
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Cache, true)
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Data, map[string]interface{}{
						"cache-true-key": "cache-true-val",
					})
				})

				it("doesn't export uncached layers", func() {
					cacheImage := h.NewFakeImage(t, "cache-image", "", "")
					err := cacher.Cache(layersDir, mockNonExistingOriginalImage, cacheImage)
					h.AssertNil(t, err)

					var cacheTrueLayer, cacheTrueNoSHALayer, otherBuildpackLayer = 1, 1, 1
					h.AssertEq(t, cacheImage.NumberOfAddedLayers(), cacheTrueLayer+cacheTrueNoSHALayer+otherBuildpackLayer)
					h.AssertEq(t, cacheImage.IsSaved(), true)
				})
			})

			when("there is a previous cached image", func() {
				var (
					fakeOriginalImage        *h.FakeImage
					computedReusableLayerSHA string
					metadataTemplate         string
				)
				it.Before(func() {
					fakeOriginalImage = h.NewFakeImage(t, "", "", "")
					computedReusableLayerSHA = "sha256:" + h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "buildpack.id/cache-true-no-sha-layer"), 1234, 4321)
					metadataTemplate = `{
  "buildpacks": [
    {
      "key": "buildpack.id",
      "layers": {
        "cache-true-layer": {
          "cache": true,
          "sha": "%s",
          "data": {"old":"data"}
        },
        "cache-true-no-sha-layer": {
          "cache": true,
          "sha": "%s"
        }
      }
    }
  ]
}`
				})
				when("the shas match", func() {
					it.Before(func() {
						h.AssertNil(t, fakeOriginalImage.SetLabel(
							"io.buildpacks.lifecycle.cache.metadata",
							fmt.Sprintf(metadataTemplate, cacheTrueLayerSHA, computedReusableLayerSHA),
						))
					})

					it("reuses layers when the calculated sha matches previous metadata", func() {
						cacheImage := h.NewFakeImage(t, "cache-image", "", "")
						err := cacher.Cache(layersDir, fakeOriginalImage, cacheImage)
						h.AssertNil(t, err)

						reusedLayers := cacheImage.ReusedLayers()
						h.AssertEq(t, len(reusedLayers), 2)
						h.AssertContains(t, reusedLayers, computedReusableLayerSHA)
						h.AssertEq(t, cacheImage.IsSaved(), true)
					})

					it("sets label metadata", func() {
						cacheImage := h.NewFakeImage(t, "cache-image", "", "")
						err := cacher.Cache(layersDir, fakeOriginalImage, cacheImage)
						h.AssertNil(t, err)

						metadataJSON, err := cacheImage.Label("io.buildpacks.lifecycle.cache.metadata")
						h.AssertNil(t, err)

						var metadata lifecycle.CacheImageMetadata
						if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
							t.Fatalf("badly formatted metadata: %s", err)
						}

						t.Log("adds layer shas to metadata label")
						h.AssertEq(t, metadata.Buildpacks[0].ID, "buildpack.id")
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].SHA, cacheTrueLayerSHA)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Launch, true)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Build, false)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Cache, true)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Data, map[string]interface{}{
							"cache-true-key": "cache-true-val",
						})

						h.AssertEq(t, metadata.Buildpacks[0].ID, "buildpack.id")
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].SHA, computedReusableLayerSHA)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].Launch, false)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].Build, false)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].Cache, true)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].Data, map[string]interface{}{
							"cache-true-no-sha-key": "cache-true-no-sha-val",
						})

						h.AssertEq(t, metadata.Buildpacks[1].ID, "other.buildpack.id")
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].SHA, otherBuildpackLayerSHA)
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].Launch, true)
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].Build, false)
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].Cache, true)
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].Data, map[string]interface{}{
							"other-buildpack-key": "other-buildpack-val",
						})
					})
				})

				when("the shas don't match", func() {
					it.Before(func() {
						h.AssertNil(t, fakeOriginalImage.SetLabel(
							"io.buildpacks.lifecycle.cache.metadata",
							fmt.Sprintf(metadataTemplate, "different-sha", "not-the-sha-you-want"),
						))
					})

					it("doesn't reuse layers", func() {
						cacheImage := h.NewFakeImage(t, "cache-image", "", "")
						err := cacher.Cache(layersDir, fakeOriginalImage, cacheImage)
						h.AssertNil(t, err)

						h.AssertEq(t, len(cacheImage.ReusedLayers()), 0)
						var cacheTrueLayer, cacheTrueNoSHALayer, otherBuildpackLayer = 1, 1, 1
						h.AssertEq(t, cacheImage.NumberOfAddedLayers(), cacheTrueLayer+cacheTrueNoSHALayer+otherBuildpackLayer)
						h.AssertEq(t, cacheImage.IsSaved(), true)
					})
				})
			})
		})

		when("there is a cache=true layer without contents", func() {
			var fakeOriginalImage *h.FakeImage

			it.Before(func() {
				layersDir = filepath.Join("testdata", "cacher", "invalid-layers")
				fakeOriginalImage = h.NewFakeImage(t, "", "", "")
				h.AssertNil(t, fakeOriginalImage.SetLabel(
					"io.buildpacks.lifecycle.cache.metadata",
					"{}"),
				)
			})

			it("fails", func() {
				cacheImage := h.NewFakeImage(t, "cache-image", "", "")
				err := cacher.Cache(layersDir, fakeOriginalImage, cacheImage)
				h.AssertError(t, err, "failed to cache layer 'buildpack.id:cache-true-no-contents' because it has no contents")
			})
		})
	})
}
