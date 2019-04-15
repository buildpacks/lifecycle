package cache_test

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle/cache"
	"github.com/buildpack/lifecycle/cache/testmock"
	"github.com/buildpack/lifecycle/image/fakes"
	"github.com/buildpack/lifecycle/metadata"
	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestImageCache(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "ImageCache", testImageCache, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testImageCache(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir            string
		fakeOriginalImage *fakes.Image
		fakeNewImage      *fakes.Image
		mockController    *gomock.Controller
		mockImageFactory  *testmock.MockImageFactory
		subject           *cache.ImageCache
		testLayerTarPath  string
		testLayerSHA      string
	)

	it.Before(func() {
		var err error

		tmpDir, err = ioutil.TempDir("", "")
		h.AssertNil(t, err)

		fakeOriginalImage = fakes.NewImage(t, "fake-image", "", "")
		fakeNewImage = fakes.NewImage(t, "fake-image", "", "")

		mockController = gomock.NewController(t)
		mockImageFactory = testmock.NewMockImageFactory(mockController)
		mockImageFactory.EXPECT().NewEmptyLocal("fake-image").Return(fakeNewImage).AnyTimes()

		subject = cache.NewImageCache(
			mockImageFactory,
			fakeOriginalImage,
		)

		testLayerTarPath = filepath.Join(tmpDir, "some-layer.tar")
		h.AssertNil(t, ioutil.WriteFile(testLayerTarPath, []byte("dummy data"), 0666))
		testLayerSHA = "sha256:" + h.ComputeSHA256ForFile(t, testLayerTarPath)
	})

	it.After(func() {
		mockController.Finish()

		os.RemoveAll(tmpDir)
	})

	when("#Name", func() {
		it("returns the image name", func() {
			h.AssertEq(t, subject.Name(), "fake-image")
		})
	})

	when("#RetrieveMetadata", func() {
		when("original image contains valid metadata", func() {
			it.Before(func() {
				h.AssertNil(t, fakeOriginalImage.SetLabel(
					"io.buildpacks.lifecycle.cache.metadata",
					`{"buildpacks": [{"key": "bp.id", "version": "1.2.3", "layers": {"some-layer": {"sha": "some-sha", "data": "some-data", "build": true, "launch": false, "cache": true}}}]}`,
				))
			})

			it("returns the metadata", func() {
				expected := cache.Metadata{
					Buildpacks: []metadata.BuildpackMetadata{{
						ID:      "bp.id",
						Version: "1.2.3",
						Layers: map[string]metadata.LayerMetadata{
							"some-layer": {
								SHA:    "some-sha",
								Data:   "some-data",
								Build:  true,
								Launch: false,
								Cache:  true,
							},
						},
					}},
				}

				meta, err := subject.RetrieveMetadata()
				h.AssertNil(t, err)
				h.AssertEq(t, meta, expected)
			})
		})

		when("original image contains invalid metadata", func() {
			it.Before(func() {
				h.AssertNil(t, fakeOriginalImage.SetLabel("io.buildpacks.lifecycle.cache.metadata", "garbage"))
			})

			it("returns empty metadata", func() {
				meta, err := subject.RetrieveMetadata()
				h.AssertNil(t, err)
				h.AssertEq(t, len(meta.Buildpacks), 0)
			})
		})

		when("original image metadata label missing", func() {
			it("returns empty metadata", func() {
				meta, err := subject.RetrieveMetadata()
				h.AssertNil(t, err)
				h.AssertEq(t, len(meta.Buildpacks), 0)
			})
		})
	})

	when("#RetrieveLayer", func() {
		when("layer exists", func() {
			it.Before(func() {
				h.AssertNil(t, fakeOriginalImage.AddLayer(testLayerTarPath))
			})

			it("returns the layer's reader", func() {
				rc, err := subject.RetrieveLayer(testLayerSHA)
				h.AssertNil(t, err)

				bytes, err := ioutil.ReadAll(rc)
				h.AssertNil(t, err)
				h.AssertEq(t, string(bytes), "dummy data")
			})
		})

		when("layer does not exist", func() {
			it("returns an error", func() {
				_, err := subject.RetrieveLayer("some_nonexistent_sha")
				h.AssertError(t, err, "failed to get layer with sha 'some_nonexistent_sha'")
			})
		})
	})

	when("#Commit", func() {
		when("with #SetMetadata", func() {
			var newMetadata cache.Metadata

			it.Before(func() {
				h.AssertNil(t, fakeOriginalImage.SetLabel("io.buildpacks.lifecycle.cache.metadata", `{"buildpacks": [{"key": "old.bp.id"}]}`))

				newMetadata = cache.Metadata{
					Buildpacks: []metadata.BuildpackMetadata{{
						ID: "new.bp.id",
					}},
				}
			})

			when("set then commit", func() {
				it("retrieve returns the newly set metadata", func() {
					h.AssertNil(t, subject.SetMetadata(newMetadata))

					err := subject.Commit()
					h.AssertNil(t, err)

					retrievedMetadata, err := subject.RetrieveMetadata()
					h.AssertNil(t, err)
					h.AssertEq(t, retrievedMetadata, newMetadata)
				})
			})

			when("set without commit", func() {
				it("retrieve returns the previous metadata", func() {
					previousMetadata := cache.Metadata{
						Buildpacks: []metadata.BuildpackMetadata{{
							ID: "old.bp.id",
						}},
					}

					h.AssertNil(t, subject.SetMetadata(newMetadata))

					retrievedMetadata, err := subject.RetrieveMetadata()
					h.AssertNil(t, err)
					h.AssertEq(t, retrievedMetadata, previousMetadata)
				})
			})
		})

		when("with #AddLayer", func() {
			when("add then commit", func() {
				it("retrieve returns newly added layer", func() {
					h.AssertNil(t, subject.AddLayer("some_identifier", testLayerSHA, testLayerTarPath))

					err := subject.Commit()
					h.AssertNil(t, err)

					rc, err := subject.RetrieveLayer(testLayerSHA)
					h.AssertNil(t, err)

					bytes, err := ioutil.ReadAll(rc)
					h.AssertNil(t, err)
					h.AssertEq(t, string(bytes), "dummy data")
				})
			})

			when("add without commit", func() {
				it("retrieve returns not found error", func() {
					h.AssertNil(t, subject.AddLayer("some_identifier", testLayerSHA, testLayerTarPath))

					_, err := subject.RetrieveLayer(testLayerSHA)
					h.AssertError(t, err, fmt.Sprintf("failed to get layer with sha '%s'", testLayerSHA))
				})
			})

		})

		when("with #ReuseLayer", func() {
			it.Before(func() {
				h.AssertNil(t, fakeOriginalImage.AddLayer(testLayerTarPath))
			})

			when("reuse then commit", func() {
				it("returns the reused layer", func() {
					h.AssertNil(t, subject.ReuseLayer("some_identifier", testLayerSHA))

					err := subject.Commit()
					h.AssertNil(t, err)

					rc, err := subject.RetrieveLayer(testLayerSHA)
					h.AssertNil(t, err)

					bytes, err := ioutil.ReadAll(rc)
					h.AssertNil(t, err)
					h.AssertEq(t, string(bytes), "dummy data")
				})
			})

			when("reuse without commit", func() {
				it("retrieve returns the previous layer", func() {
					h.AssertNil(t, subject.ReuseLayer("some_identifier", testLayerSHA))

					rc, err := subject.RetrieveLayer(testLayerSHA)
					h.AssertNil(t, err)

					bytes, err := ioutil.ReadAll(rc)
					h.AssertNil(t, err)
					h.AssertEq(t, string(bytes), "dummy data")
				})
			})

		})
	})
}
