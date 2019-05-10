package lifecycle_test

import (
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/buildpack/imgutil"
	"github.com/buildpack/imgutil/fakes"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cache"
	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestCachingImage(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Exporter", testCachingImage, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCachingImage(t *testing.T, when spec.G, it spec.S) {
	var (
		subject     imgutil.Image
		fakeImage   *fakes.Image
		volumeCache *cache.VolumeCache
		tmpDir      string
		layerPath   string
		layerSHA    string
		layerData   []byte
	)

	it.Before(func() {
		var err error
		fakeImage = fakes.NewImage("some-image", "", "")
		tmpDir, err = ioutil.TempDir("", "")
		h.AssertNil(t, err)
		volumeCache, err = cache.NewVolumeCache(tmpDir)
		h.AssertNil(t, err)
		subject = lifecycle.NewCachingImage(fakeImage, volumeCache)
		layerPath, layerSHA, layerData = h.RandomLayer(t, tmpDir)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#AddLayerFile", func() {
		it("adds the layer to the cache and the image", func() {
			h.AssertNil(t, subject.AddLayer(layerPath))

			_, err := subject.Save()
			h.AssertNil(t, err)

			_, err = fakeImage.GetLayer(layerSHA)
			h.AssertNil(t, err)

			_, err = volumeCache.RetrieveLayer(layerSHA)
			h.AssertNil(t, err)
		})
	})

	when("#ReuseLayer", func() {
		when("the layer exists in the cache", func() {
			it.Before(func() {
				from, err := os.Open(layerPath)
				h.AssertNil(t, err)
				defer from.Close()
				to, err := os.Create(filepath.Join(tmpDir, "committed", layerSHA+".tar"))
				h.AssertNil(t, err)
				defer to.Close()
				_, err = io.Copy(to, from)
				h.AssertNil(t, err)
			})

			it("adds the layer from the cache to the image", func() {
				h.AssertNil(t, subject.ReuseLayer(layerSHA))

				_, err := subject.Save()
				h.AssertNil(t, err)

				_, err = fakeImage.GetLayer(layerSHA)
				h.AssertNil(t, err)
			})

			it("keeps the layer in the cache", func() {
				h.AssertNil(t, subject.ReuseLayer(layerSHA))

				_, err := subject.Save()
				h.AssertNil(t, err)

				_, err = volumeCache.RetrieveLayerFile(layerSHA)
				h.AssertNil(t, err)
			})
		})

		when("the layer does not exist in the cache", func() {
			it.Before(func() {
				fakeImage.AddPreviousLayer(layerSHA, layerPath)
			})

			it("reuses the layer from the image", func() {
				h.AssertNil(t, subject.ReuseLayer(layerSHA))

				_, err := subject.Save()
				h.AssertNil(t, err)

				for _, reusedSHA := range fakeImage.ReusedLayers() {
					if reusedSHA == layerSHA {
						return
					}
				}
				t.Fatalf("expected image to have reused layer '%s'", layerSHA)
			})

			it("adds the layer to the cache", func() {
				h.AssertNil(t, subject.ReuseLayer(layerSHA))

				_, err := subject.Save()
				h.AssertNil(t, err)

				_, err = volumeCache.RetrieveLayer(layerSHA)
				h.AssertNil(t, err)
			})
		})
	})

	when("#GetLayer", func() {
		when("the layer exists in the cache", func() {
			it.Before(func() {
				h.AssertNil(t, volumeCache.AddLayerFile(layerSHA, layerPath))
				h.AssertNil(t, volumeCache.Commit())
			})

			it("gets it from the cache", func() {
				rc, err := subject.GetLayer(layerSHA)
				h.AssertNil(t, err)
				defer rc.Close()
				contents, err := ioutil.ReadAll(rc)
				h.AssertNil(t, err)
				h.AssertEq(t, contents, layerData)
			})
		})

		when("the layer does not exist in the cache", func() {
			it.Before(func() {
				h.AssertNil(t, fakeImage.AddLayer(layerPath))
				_, err := fakeImage.Save()
				h.AssertNil(t, err)
			})

			it("gets it from the image", func() {
				rc, err := subject.GetLayer(layerSHA)
				h.AssertNil(t, err)
				defer rc.Close()
				contents, err := ioutil.ReadAll(rc)
				h.AssertNil(t, err)
				h.AssertEq(t, contents, layerData)
			})
		})
	})
}
