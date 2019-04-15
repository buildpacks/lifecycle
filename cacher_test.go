package lifecycle_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/cache"
	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestCacher(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Cacher", testCacher, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCacher(t *testing.T, when spec.G, it spec.S) {
	when("#Cacher", func() {
		var (
			tmpDir                 string
			cacheDir               string
			testCache              lifecycle.Cache
			layersDir              string
			cacheTrueLayerSHA      string
			otherBuildpackLayerSHA string
			subject                *lifecycle.Cacher
		)

		it.Before(func() {
			var err error

			emptyLogger := log.New(ioutil.Discard, "", 0)

			tmpDir, err = ioutil.TempDir("", "lifecycle.cacher.layer")
			h.AssertNil(t, err)

			cacheDir, err = ioutil.TempDir("", "")
			h.AssertNil(t, err)

			testCache, err = cache.NewVolumeCache(cacheDir)
			h.AssertNil(t, err)

			subject = &lifecycle.Cacher{
				ArtifactsDir: tmpDir,
				Buildpacks: []*lifecycle.Buildpack{
					{ID: "buildpack.id"},
					{ID: "other.buildpack.id"},
				},
				Out: emptyLogger,
				UID: 1234,
				GID: 4321,
			}
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(cacheDir))
			h.AssertNil(t, os.RemoveAll(tmpDir))
		})

		when("the layers are valid", func() {
			it.Before(func() {
				layersDir = filepath.Join("testdata", "cacher", "layers")
				cacheTrueLayerSHA = "sha256:" + h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "buildpack.id/cache-true-layer"), 1234, 4321)
				otherBuildpackLayerSHA = "sha256:" + h.ComputeSHA256ForPath(t, filepath.Join(layersDir, "other.buildpack.id/other-buildpack-layer"), 1234, 4321)
			})

			when("there is no previous cache", func() {
				it("adds layers with 'cache=true' to the cache", func() {
					err := subject.Cache(layersDir, testCache)
					h.AssertNil(t, err)

					assertTarFileContents(
						t,
						filepath.Join(cacheDir, "committed", cacheTrueLayerSHA+".tar"),
						filepath.Join(layersDir, "buildpack.id/cache-true-layer/file-from-cache-true-layer"),
						"file-from-cache-true-contents",
					)

					assertTarFileContents(
						t,
						filepath.Join(cacheDir, "committed", otherBuildpackLayerSHA+".tar"),
						filepath.Join(layersDir, "other.buildpack.id/other-buildpack-layer/other-buildpack-layer-file"),
						"other-buildpack-layer-contents",
					)
				})

				it("sets the uid and gid of the layer contents", func() {
					err := subject.Cache(layersDir, testCache)
					h.AssertNil(t, err)

					assertTarFileOwner(
						t,
						filepath.Join(cacheDir, "committed", cacheTrueLayerSHA+".tar"),
						filepath.Join(layersDir, "buildpack.id/cache-true-layer/file-from-cache-true-layer"),
						1234,
						4321,
					)

					assertTarFileOwner(
						t,
						filepath.Join(cacheDir, "committed", otherBuildpackLayerSHA+".tar"),
						filepath.Join(layersDir, "other.buildpack.id/other-buildpack-layer/other-buildpack-layer-file"),
						1234,
						4321,
					)
				})

				it("sets cache metadata", func() {
					err := subject.Cache(layersDir, testCache)
					h.AssertNil(t, err)

					metadata, err := testCache.RetrieveMetadata()
					h.AssertNil(t, err)

					t.Log("adds layer shas to metadata")
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
					err := subject.Cache(layersDir, testCache)
					h.AssertNil(t, err)

					matches, err := filepath.Glob(filepath.Join(cacheDir, "committed", "*.tar"))
					h.AssertNil(t, err)
					h.AssertEq(t, len(matches), 3)
				})
			})

			when("there are previously cached layers", func() {
				var (
					computedReusableLayerSHA string
					metadataTemplate         string
				)

				it.Before(func() {
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

				when("the SHAs match", func() {
					it.Before(func() {
						err := subject.Cache(layersDir, testCache)
						h.AssertNil(t, err)
					})

					it("reuses layers when the calculated sha matches previous metadata", func() {
						previousLayers, err := filepath.Glob(filepath.Join(cacheDir, "committed", "*.tar"))
						h.AssertNil(t, err)

						err = subject.Cache(layersDir, testCache)
						h.AssertNil(t, err)

						reusedLayers, err := filepath.Glob(filepath.Join(cacheDir, "committed", "*.tar"))
						h.AssertNil(t, err)

						h.AssertEq(t, previousLayers, reusedLayers)
					})

					it("sets cache metadata", func() {
						err := subject.Cache(layersDir, testCache)
						h.AssertNil(t, err)

						metadata, err := testCache.RetrieveMetadata()
						h.AssertNil(t, err)

						t.Log("adds layer shas to metadata")
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
						err := ioutil.WriteFile(
							filepath.Join(cacheDir, "committed", "io.buildpacks.lifecycle.cache.metadata"),
							[]byte(fmt.Sprintf(metadataTemplate, "different-sha", "not-the-sha-you-want")),
							0666,
						)
						h.AssertNil(t, err)

						err = ioutil.WriteFile(
							filepath.Join(cacheDir, "committed", "some-layer.tar"),
							[]byte("some data"),
							0666,
						)
						h.AssertNil(t, err)
					})

					it("doesn't reuse layers", func() {
						err := subject.Cache(layersDir, testCache)
						h.AssertNil(t, err)

						matches, err := filepath.Glob(filepath.Join(cacheDir, "committed", "*.tar"))
						h.AssertNil(t, err)
						h.AssertEq(t, len(matches), 3)

						for _, m := range matches {
							if strings.Contains(m, "some-layer.tar") {
								t.Fatal("expected layer 'some-layer.tar' not to exist")
							}
						}
					})
				})
			})
		})

		when("there is a cache=true layer without contents", func() {
			it.Before(func() {
				layersDir = filepath.Join("testdata", "cacher", "invalid-layers")

				err := ioutil.WriteFile(
					filepath.Join(cacheDir, "committed", "io.buildpacks.lifecycle.cache.metadata"),
					[]byte("{}"),
					0666,
				)
				h.AssertNil(t, err)
			})

			it("fails", func() {
				err := subject.Cache(layersDir, testCache)
				h.AssertError(t, err, "failed to cache layer 'buildpack.id:cache-true-no-contents' because it has no contents")
			})
		})
	})
}
