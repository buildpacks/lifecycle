package phase_test

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/apex/log/handlers/memory"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/phase/testmock"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestCache(t *testing.T) {
	spec.Run(t, "Exporter", testCache, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCache(t *testing.T, when spec.G, it spec.S) {
	when("#Cache", func() {
		var (
			cacheDir     string
			exporter     *phase.Exporter
			layerFactory *testmock.MockLayerFactory
			layersDir    string
			logHandler   *memory.Handler
			mockCtrl     *gomock.Controller
			testCache    phase.Cache
			tmpDir       string
		)

		it.Before(func() {
			var err error
			mockCtrl = gomock.NewController(t)
			layerFactory = testmock.NewMockLayerFactory(mockCtrl)

			logHandler = memory.New()
			level, err := log.ParseLevel("info")
			h.AssertNil(t, err)

			tmpDir, err = os.MkdirTemp("", "lifecycle.cacher.layer")
			h.AssertNil(t, err)
			h.AssertNil(t, os.Mkdir(filepath.Join(tmpDir, "artifacts"), 0777))

			cacheDir = filepath.Join(tmpDir, "cache")
			h.AssertNil(t, os.Mkdir(cacheDir, 0777))

			testCache, err = cache.NewVolumeCache(cacheDir, &log.Logger{Handler: logHandler, Level: level})
			h.AssertNil(t, err)

			exporter = &phase.Exporter{
				PlatformAPI: api.Platform.Latest(),
				Buildpacks: []buildpack.GroupElement{
					{ID: "buildpack.id", API: api.Buildpack.Latest().String()},
					{ID: "other.buildpack.id", API: api.Buildpack.Latest().String()},
				},
				Logger:       &log.Logger{Handler: logHandler, Level: level},
				LayerFactory: layerFactory,
			}
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tmpDir))
			mockCtrl.Finish()
		})

		when("the layers are valid", func() {
			it.Before(func() {
				layerFactory.EXPECT().
					DirLayer(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(id string, dir string, createdBy string) (layers.Layer, error) {
						return createTestCacheLayer(id, tmpDir)
					}).AnyTimes()

				layersDir = filepath.Join("testdata", "cacher", "layers")
			})

			when("there is no previous cache", func() {
				it("adds layers with 'cache=true' to the cache", func() {
					err := exporter.Cache(layersDir, testCache)
					h.AssertNil(t, err)

					assertCacheHasLayer(t, testCache, "buildpack.id:cache-true-layer")
					assertCacheHasLayer(t, testCache, "other.buildpack.id:other-buildpack-layer")
				})

				it("sets cache metadata", func() {
					err := exporter.Cache(layersDir, testCache)
					h.AssertNil(t, err)

					metadata, err := testCache.RetrieveMetadata()
					h.AssertNil(t, err)

					t.Log("adds layer shas to metadata")
					h.AssertEq(t, metadata.Buildpacks[0].ID, "buildpack.id")
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].SHA, testCacheLayerDigest("buildpack.id:cache-true-layer"))
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Launch, true)
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Build, false)
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Cache, true)
					h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Data, map[string]any{
						"cache-true-key": "cache-true-val",
					})
				})

				it("doesn't export uncached layers", func() {
					err := exporter.Cache(layersDir, testCache)
					h.AssertNil(t, err)

					matches, err := filepath.Glob(filepath.Join(cacheDir, "committed", "*.tar"))
					h.AssertNil(t, err)
					h.AssertEq(t, len(matches), 4)
				})
			})

			when("structured SBOM", func() {
				when("there is a 'cache=true' layer with a bom.<ext> file", func() {
					it("adds the bom.<ext> file to the cache", func() {
						err := exporter.Cache(layersDir, testCache)
						h.AssertNil(t, err)

						metadata, err := testCache.RetrieveMetadata()
						h.AssertNil(t, err)

						t.Log("adds bom sha to metadata")
						h.AssertEq(t, metadata.BOM.SHA, testCacheLayerDigest("cache.sbom"))
						assertCacheHasLayer(t, testCache, "cache.sbom")
					})
				})
			})

			when("there are previously cached layers", func() {
				var (
					metadataTemplate string
				)

				it.Before(func() {
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
						initializeCache(t, exporter, &testCache, cacheDir, layersDir, metadataTemplate)
					})

					it("reuses layers when the calculated sha matches previous metadata", func() {
						previousLayers, err := filepath.Glob(filepath.Join(cacheDir, "committed", "*.tar"))
						h.AssertNil(t, err)

						err = exporter.Cache(layersDir, testCache)
						h.AssertNil(t, err)

						reusedLayers, err := filepath.Glob(filepath.Join(cacheDir, "committed", "*.tar"))
						h.AssertNil(t, err)

						h.AssertEq(t, previousLayers, reusedLayers)
					})

					it("does not also add a layer it just reused", func() {
						// simulates an image cache, whose VerifyLayer is a no-op that always succeeds
						wrappedCache := &verifyAlwaysCache{Cache: testCache}

						err := exporter.Cache(layersDir, wrappedCache)
						h.AssertNil(t, err)

						for _, sha := range wrappedCache.addedLayerSHAs {
							if sha == testCacheLayerDigest("cache-true-layer") {
								t.Fatalf("expected reused layer '%s' not to also be added", testCacheLayerDigest("cache-true-layer"))
							}
						}
					})

					it("sets cache metadata", func() {
						err := exporter.Cache(layersDir, testCache)
						h.AssertNil(t, err)

						metadata, err := testCache.RetrieveMetadata()
						h.AssertNil(t, err)

						t.Log("adds layer shas to metadata")
						h.AssertEq(t, metadata.Buildpacks[0].ID, "buildpack.id")
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].SHA, testCacheLayerDigest("cache-true-layer"))
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Launch, true)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Build, false)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Cache, true)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-layer"].Data, map[string]any{
							"cache-true-key": "cache-true-val",
						})

						h.AssertEq(t, metadata.Buildpacks[0].ID, "buildpack.id")
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].SHA, testCacheLayerDigest("cache-true-no-sha-layer"))
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].Launch, false)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].Build, false)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].Cache, true)
						h.AssertEq(t, metadata.Buildpacks[0].Layers["cache-true-no-sha-layer"].Data, map[string]any{
							"cache-true-no-sha-key": "cache-true-no-sha-val",
						})

						h.AssertEq(t, metadata.Buildpacks[1].ID, "other.buildpack.id")
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].SHA, testCacheLayerDigest("other-buildpack-layer"))
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].Launch, true)
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].Build, false)
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].Cache, true)
						h.AssertEq(t, metadata.Buildpacks[1].Layers["other-buildpack-layer"].Data, map[string]any{
							"other-buildpack-key": "other-buildpack-val",
						})
					})
				})

				when("the shas don't match", func() {
					it.Before(func() {
						err := os.WriteFile(
							filepath.Join(cacheDir, "committed", "io.buildpacks.lifecycle.cache.metadata"),
							fmt.Appendf(nil, metadataTemplate, testCacheLayerDigest("different-sha"), testCacheLayerDigest("not-the-sha-you-want")),
							0600,
						)
						h.AssertNil(t, err)

						err = os.WriteFile(
							filepath.Join(cacheDir, "committed", testCacheLayerDigest("different-sha")+".tar"),
							[]byte("some data"),
							0600,
						)
						h.AssertNil(t, err)
					})

					it("doesn't reuse layers", func() {
						err := exporter.Cache(layersDir, testCache)
						h.AssertNil(t, err)

						matches, err := filepath.Glob(filepath.Join(cacheDir, "committed", "*.tar"))
						h.AssertNil(t, err)
						h.AssertEq(t, len(matches), 4)

						for _, m := range matches {
							if strings.Contains(m, testCacheLayerDigest("different-sha")) {
								t.Fatal("expected layer with different sha not to exist")
							}
						}
					})
				})
			})
		})

		when("there are invalid layers", func() {
			it.Before(func() {
				layerFactory.EXPECT().
					DirLayer("buildpack.id:layer-1", gomock.Any(), gomock.Any()).
					Return(layers.Layer{}, errors.New("test error"))
				layerFactory.EXPECT().
					DirLayer("buildpack.id:layer-2", gomock.Any(), gomock.Any()).
					DoAndReturn(func(id string, dir string, createdBy string) (layers.Layer, error) {
						return createTestCacheLayer(id, tmpDir)
					}).
					AnyTimes()
				layersDir = filepath.Join("testdata", "cacher", "invalid-layers")
				h.AssertNil(t, exporter.Cache(layersDir, testCache))
				h.AssertEq(t, len(logHandler.Entries), 3)
			})

			it("warns when there is a cache=true layer without contents", func() {
				h.AssertStringContains(t, logHandler.Entries[0].Message, "Failed to cache layer 'buildpack.id:cache-true-no-contents' because it has no contents")
			})

			it("warns when there is an error adding a layer", func() {
				h.AssertStringContains(t, logHandler.Entries[1].Message, "Failed to cache layer 'buildpack.id:layer-1': creating layer 'buildpack.id:layer-1': test error")
			})

			it("continues caching valid layers", func() {
				h.AssertStringContains(t, logHandler.Entries[2].Message, "Adding cache layer 'buildpack.id:layer-2'")
				assertCacheHasLayer(t, testCache, "buildpack.id:layer-2")
			})
		})
	})
}

// verifyAlwaysCache wraps a phase.Cache and makes VerifyLayer always succeed,
// mirroring cache.ImageCache#VerifyLayer, which trusts the registry to have
// already verified digests. It also records every SHA passed to AddLayerFile.
type verifyAlwaysCache struct {
	phase.Cache
	addedLayerSHAs []string
}

func (c *verifyAlwaysCache) VerifyLayer(_ string) error {
	return nil
}

func (c *verifyAlwaysCache) AddLayerFile(tarPath, sha string) error {
	c.addedLayerSHAs = append(c.addedLayerSHAs, sha)
	return c.Cache.AddLayerFile(tarPath, sha)
}

func testCacheLayerDigest(id string) string {
	parts := strings.Split(id, ":")
	last := parts[len(parts)-1]
	sum := sha256.Sum256([]byte(last))
	return fmt.Sprintf("sha256:%x", sum)
}

func createTestCacheLayer(id string, tmpDir string) (layers.Layer, error) {
	tarPath := filepath.Join(tmpDir, "artifacts", strings.ReplaceAll(id, "/", "_"))
	f, err := os.Create(tarPath) //nolint:gosec
	if err != nil {
		return layers.Layer{}, err
	}
	defer func() {
		_ = f.Close()
	}()
	_, err = f.Write([]byte(testLayerContents(id)))
	if err != nil {
		return layers.Layer{}, err
	}
	return layers.Layer{
		ID:      id,
		TarPath: tarPath,
		Digest:  testCacheLayerDigest(id),
	}, nil
}

func assertCacheHasLayer(t *testing.T, cache phase.Cache, id string) {
	t.Helper()

	rc, err := cache.RetrieveLayer(testCacheLayerDigest(id))
	h.AssertNil(t, err)
	defer rc.Close()
	contents, err := io.ReadAll(rc)
	h.AssertNil(t, err)
	h.AssertEq(t, string(contents), testLayerContents(id))
}

func initializeCache(t *testing.T, exporter *phase.Exporter, testCache *phase.Cache, cacheDir, layersDir, metadataTemplate string) {
	logger := &log.Logger{Handler: &discard.Handler{}}

	previousCache, err := cache.NewVolumeCache(cacheDir, logger)
	h.AssertNil(t, err)

	err = exporter.Cache(layersDir, previousCache)
	h.AssertNil(t, err)

	*testCache, err = cache.NewVolumeCache(cacheDir, logger)
	h.AssertNil(t, err)

	h.AssertNil(t, os.WriteFile(
		filepath.Join(cacheDir, "committed", "io.buildpacks.lifecycle.cache.metadata"),
		fmt.Appendf(nil, metadataTemplate, testCacheLayerDigest("cache-true-layer"), "cache-true-no-sha-layer"),
		0600,
	))
}
