package lifecycle_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func testRestorer07(t *testing.T, when spec.G, it spec.S) {
	when("#Restore", func() {
		var (
			layersDir  string
			cacheDir   string
			skipLayers bool
			testCache  lifecycle.Cache
			restorer   *lifecycle.Restorer
		)

		it.Before(func() {
			var err error

			layersDir, err = ioutil.TempDir("", "lifecycle-layer-dir")
			h.AssertNil(t, err)

			cacheDir, err = ioutil.TempDir("", "")
			h.AssertNil(t, err)

			testCache, err = cache.NewVolumeCache(cacheDir)
			h.AssertNil(t, err)

			platform := platform.NewPlatform("0.7")
			discardLogger := log.Logger{Handler: &discard.Handler{}}
			cacheMetadataRetriever := &lifecycle.DefaultCacheMetadataRetriever{
				Logger: &discardLogger,
			}
			restorer = &lifecycle.Restorer{
				LayersDir: layersDir,
				Buildpacks: []buildpack.GroupBuildpack{
					{ID: "buildpack.id", API: api.Buildpack.Latest().String()},
					{ID: "escaped/buildpack/id", API: api.Buildpack.Latest().String()},
				},
				Logger: &discardLogger,
				LayerAnalyzer: lifecycle.NewLayerAnalyzer(
					&discardLogger,
					cacheMetadataRetriever,
					layersDir,
					platform,
					skipLayers),
				CacheMetadataRetriever: cacheMetadataRetriever,
				Platform:               platform,
			}
			if testing.Verbose() {
				restorer.Logger = cmd.DefaultLogger
				h.AssertNil(t, cmd.SetLogLevel("debug"))
			}
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(layersDir))
			h.AssertNil(t, os.RemoveAll(cacheDir))
		})

		when("there is no cache", func() {
			when("there is a cache=true layer", func() {
				it.Before(func() {
					meta := "[types]\n  cache=true"
					h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-true", meta, "cache-only-layer-sha"))
					h.AssertNil(t, restorer.Restore(nil))
				})

				it("removes metadata and sha file", func() {
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-true.toml"))
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-true.sha"))
				})
				it("does not restore layer data", func() {
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-true"))
				})
			})
			when("there is a cache=false layer", func() {
				it.Before(func() {
					meta := "[types]\n  cache=false"
					h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-false", meta, "cache-false-layer-sha"))
					h.AssertNil(t, restorer.Restore(testCache))
				})

				it("keeps metadata and sha file", func() {
					h.AssertPathExists(t, filepath.Join(layersDir, "buildpack.id", "cache-false.toml"))
					h.AssertPathExists(t, filepath.Join(layersDir, "buildpack.id", "cache-false.sha"))
				})
				it("does not restore layer data", func() {
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-false"))
				})
			})
		})

		when("there is an empty cache", func() {
			when("there is a cache=true layer", func() {
				it.Before(func() {
					meta := "[types]\n  cache=true"
					h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-true", meta, "cache-only-layer-sha"))
					h.AssertNil(t, restorer.Restore(testCache))
				})

				it("removes metadata and sha file", func() {
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-true.toml"))
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-true.sha"))
				})
				it("does not restore layer data", func() {
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-true"))
				})
			})
			when("there is a cache=false layer", func() {
				it.Before(func() {
					meta := "[types]\n  cache=false"
					h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-false", meta, "cache-false-layer-sha"))
					h.AssertNil(t, restorer.Restore(testCache))
				})

				it("keeps metadata and sha file", func() {
					h.AssertPathExists(t, filepath.Join(layersDir, "buildpack.id", "cache-false.toml"))
					h.AssertPathExists(t, filepath.Join(layersDir, "buildpack.id", "cache-false.sha"))
				})
				it("does not restore layer data", func() {
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-false"))
				})
			})
		})

		when("there is a cache", func() {
			var (
				tarTempDir          string
				cacheOnlyLayerSHA   string
				cacheLaunchLayerSHA string
				noGroupLayerSHA     string
				cacheFalseLayerSHA  string
				escapedLayerSHA     string
			)

			it.Before(func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "restorer"), layersDir)
				var err error

				tarTempDir, err = ioutil.TempDir("", "restorer-test-temp-layer")
				h.AssertNil(t, err)

				lf := layers.Factory{
					ArtifactsDir: tarTempDir,
					Logger:       nil,
				}
				layer, err := lf.DirLayer("buildpack.id:cache-only", filepath.Join(layersDir, "buildpack.id", "cache-only"))
				h.AssertNil(t, err)
				cacheOnlyLayerSHA = layer.Digest
				h.AssertNil(t, testCache.AddLayerFile(layer.TarPath, layer.Digest))

				layer, err = lf.DirLayer("buildpack.id:cache-false", filepath.Join(layersDir, "buildpack.id", "cache-false"))
				h.AssertNil(t, err)
				cacheFalseLayerSHA = layer.Digest
				h.AssertNil(t, testCache.AddLayerFile(layer.TarPath, layer.Digest))

				layer, err = lf.DirLayer("buildpack.id:cache-launch", filepath.Join(layersDir, "buildpack.id", "cache-launch"))
				h.AssertNil(t, err)
				cacheLaunchLayerSHA = layer.Digest
				h.AssertNil(t, testCache.AddLayerFile(layer.TarPath, layer.Digest))

				layer, err = lf.DirLayer("nogroup.buildpack.id:some-layer", filepath.Join(layersDir, "nogroup.buildpack.id", "some-layer"))
				h.AssertNil(t, err)
				noGroupLayerSHA = layer.Digest
				h.AssertNil(t, testCache.AddLayerFile(layer.TarPath, layer.Digest))

				layer, err = lf.DirLayer("escaped/buildpack/id.id:escaped-bp-layer", filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer"))
				h.AssertNil(t, err)
				escapedLayerSHA = layer.Digest
				h.AssertNil(t, testCache.AddLayerFile(layer.TarPath, layer.Digest))

				h.AssertNil(t, testCache.Commit())
				h.AssertNil(t, os.RemoveAll(layersDir))
				h.AssertNil(t, os.Mkdir(layersDir, 0777))

				contents := fmt.Sprintf(`{
    "buildpacks": [
        {
            "key": "buildpack.id",
            "layers": {
                "cache-false": {
                    "cache": false,
                    "sha": "%s"
                },
                "cache-launch": {
                    "cache": true,
                    "launch": true,
                    "sha": "%s"
                },
                "cache-only": {
                    "cache": true,
                    "data": {
                        "some-key": "some-value"
                    },
                    "sha": "%s"
                }
            }
        },
        {
            "key": "nogroup.buildpack.id",
            "layers": {
                "some-layer": {
                    "cache": true,
                    "sha": "%s"
                }
            }
        },
        {
            "key": "escaped/buildpack/id",
            "layers": {
                "escaped-bp-layer": {
                    "cache": true,
                    "sha": "%s"
                }
            }
        }
    ]
}
`, cacheFalseLayerSHA, cacheLaunchLayerSHA, cacheOnlyLayerSHA, noGroupLayerSHA, escapedLayerSHA)

				err = ioutil.WriteFile(
					filepath.Join(cacheDir, "committed", "io.buildpacks.lifecycle.cache.metadata"),
					[]byte(contents),
					0600,
				)
				h.AssertNil(t, err)
			})

			it.After(func() {
				h.AssertNil(t, os.RemoveAll(tarTempDir))
			})

			when("there is a cache=false layer", func() {
				var meta string
				it.Before(func() {
					meta = "[types]\n  cache=false\n[metadata]\n  cache-false-key = \"cache-false-val\""
					h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-false", meta, cacheFalseLayerSHA))
					h.AssertNil(t, restorer.Restore(testCache))
				})

				it("keeps layer metadatata", func() {
					got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-false.toml"))
					h.AssertEq(t, string(got), meta)
				})
				it("keeps layer sha", func() {
					got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-false.sha"))
					h.AssertEq(t, string(got), cacheFalseLayerSHA)
				})
				it("does not restore data", func() {
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-false"))
				})
			})

			when("there is a cache=true layer with wrong sha", func() {
				it.Before(func() {
					meta := "[types]\n  cache=true"
					h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-true", meta, "some-made-up-sha"))
					h.AssertNil(t, restorer.Restore(testCache))
				})

				it("removes metadata and sha file", func() {
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-true.toml"))
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-true.sha"))
				})
				it("does not restore layer data", func() {
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-true"))
				})
			})

			when("there is a cache=true layer not in cache", func() {
				it.Before(func() {
					meta := "[types]\n  cache=true"
					h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-layer-not-in-cache", meta, "some-made-up-sha"))
					h.AssertNil(t, restorer.Restore(testCache))
				})

				it("removes metadata and sha file", func() {
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-layer-not-in-cache.toml"))
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-layer-not-in-cache.sha"))
				})
				it("does not restore layer data", func() {
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-layer-not-in-cache"))
				})
			})

			when("there is a cache=true layer in cache but not in group", func() {
				it("analyzes the layers with the provided buildpacks", func() {
					meta := "cache=true"
					h.AssertNil(t, writeLayer(layersDir, "nogroup.buildpack.id", "some-layer", meta, noGroupLayerSHA))
					h.AssertNil(t, restorer.Restore(testCache))
				})
			})
		})

		when("there is app image metadata", func() {
			it.Before(func() {
				restorer.Buildpacks = []buildpack.GroupBuildpack{
					{ID: "metadata.buildpack", API: api.Buildpack.Latest().String()},
					{ID: "no.cache.buildpack", API: api.Buildpack.Latest().String()},
					{ID: "no.metadata.buildpack", API: api.Buildpack.Latest().String()},
				}
				metadata := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "app_metadata.json"))
				h.AssertNil(t, json.Unmarshal(metadata, &restorer.LayersMetadata))
			})

			it("restores layer metadata and unsets the launch, build and cache flags", func() {
				err := restorer.Restore(testCache)
				h.AssertNil(t, err)

				unsetFlags := "[types]"
				for _, data := range []struct{ name, want string }{
					{"metadata.buildpack/launch.toml", "[metadata]\n  launch-key = \"launch-value\""},
					{"metadata.buildpack/launch-build-cache.toml", "[metadata]\n  launch-build-cache-key = \"launch-build-cache-value\""},
					{"metadata.buildpack/launch-cache.toml", "[metadata]\n  launch-cache-key = \"launch-cache-value\""},
					{"no.cache.buildpack/some-layer.toml", "[metadata]\n  some-layer-key = \"some-layer-value\""},
				} {
					got := h.MustReadFile(t, filepath.Join(layersDir, data.name))
					h.AssertStringContains(t, string(got), data.want)
					h.AssertStringDoesNotContain(t, string(got), unsetFlags) // The [types] table shouldn't exist. The build, cache and launch flags are set to false.
				}
			})

			it("restores layer sha files", func() {
				err := restorer.Restore(testCache)
				h.AssertNil(t, err)

				for _, data := range []struct{ name, want string }{
					{"metadata.buildpack/launch.sha", "launch-sha"},
					{"metadata.buildpack/launch-build-cache.sha", "launch-build-cache-sha"},
					{"metadata.buildpack/launch-cache.sha", "launch-cache-sha"},
					{"no.cache.buildpack/some-layer.sha", "some-layer-sha"},
				} {
					got := h.MustReadFile(t, filepath.Join(layersDir, data.name))
					h.AssertStringContains(t, string(got), data.want)
				}
			})

			it("does not restore launch=false layer metadata", func() {
				err := restorer.Restore(testCache)
				h.AssertNil(t, err)

				h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "launch-false.toml"))
				h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "launch-false.sha"))
			})

			it("does not restore build=true, cache=false layer metadata", func() {
				err := restorer.Restore(testCache)
				h.AssertNil(t, err)

				h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "launch-build.sha"))
			})

			when("subset of buildpacks are detected", func() {
				it.Before(func() {
					restorer.Buildpacks = []buildpack.GroupBuildpack{{ID: "no.cache.buildpack", API: api.Buildpack.Latest().String()}}
				})
				it("restores layers for detected buildpacks", func() {
					err := restorer.Restore(testCache)
					h.AssertNil(t, err)

					path := filepath.Join(layersDir, "no.cache.buildpack", "some-layer.toml")
					got := h.MustReadFile(t, path)
					want := "[metadata]\n  some-layer-key = \"some-layer-value\""

					h.AssertStringContains(t, string(got), want)
				})
				it("does not restore layers for undetected buildpacks", func() {
					err := restorer.Restore(testCache)
					h.AssertNil(t, err)

					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack"))
				})
			})

			it("restores each store metadata", func() {
				err := restorer.Restore(testCache)
				h.AssertNil(t, err)
				for _, data := range []struct{ name, want string }{
					// store.toml files.
					{"metadata.buildpack/store.toml", "[metadata]\n  [metadata.metadata-buildpack-store-data]\n    store-key = \"store-val\""},
					{"no.cache.buildpack/store.toml", "[metadata]\n  [metadata.no-cache-buildpack-store-data]\n    store-key = \"store-val\""},
				} {
					got := h.MustReadFile(t, filepath.Join(layersDir, data.name))
					h.AssertStringContains(t, string(got), data.want)
				}
			})
		})

		when("there is app image metadata", func() {
			it.Before(func() {
				restorer.LayersMetadata = platform.LayersMetadata{}
			})

			it("analyzes with no layer metadata", func() {
				err := restorer.Restore(testCache)
				h.AssertNil(t, err)
			})
		})
	})
}
