package layer_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestLayerMetadataRestorer(t *testing.T) {
	spec.Run(t, "MetadataRestorer", testLayerMetadataRestorer, spec.Report(report.Terminal{}))
}

func testLayerMetadataRestorer(t *testing.T, when spec.G, it spec.S) {
	var (
		layerDir              string
		layerMetadataRestorer layer.MetadataRestorer
		layerSHAStore         layer.SHAStore
		layersMetadata        files.LayersMetadata
		cacheMetadata         platform.CacheMetadata
		buildpacks            []buildpack.GroupElement
		skipLayers            bool
		logger                log.Logger
	)

	it.Before(func() {
		var err error

		layerDir, err = os.MkdirTemp("", "lifecycle-layer-dir")
		h.AssertNil(t, err)
		logger = log.Logger{Handler: &discard.Handler{}}
		layerMetadataRestorer = layer.NewDefaultMetadataRestorer(layerDir, skipLayers, &logger, api.Platform.Latest())
		layerSHAStore = layer.NewSHAStore()
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(layerDir))
	})

	when("#Restore", func() {
		it.Before(func() {
			buildpacks = []buildpack.GroupElement{
				{ID: "metadata.buildpack", API: api.Buildpack.Latest().String()},
				{ID: "no.cache.buildpack", API: api.Buildpack.Latest().String()},
				{ID: "escaped/buildpack/id", API: api.Buildpack.Latest().String()},
			}
		})

		when("app and cache metadata are not present", func() {
			it("does not restore any metadata", func() {
				err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
				h.AssertNil(t, err)

				files, err := os.ReadDir(layerDir)
				h.AssertNil(t, err)
				h.AssertEq(t, len(files), 0)
			})
		})

		when("only app metadata is present", func() {
			it.Before(func() {
				layerMetaDataJSON := h.MustReadFile(t, filepath.Join("testdata", "app_metadata.json"))
				h.AssertNil(t, json.Unmarshal(layerMetaDataJSON, &layersMetadata))
			})

			it("restores each store metadata", func() {
				err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
				h.AssertNil(t, err)

				files, err := os.ReadDir(layerDir)
				h.AssertNil(t, err)
				h.AssertEq(t, len(files), 2)

				for _, data := range []struct{ name, want string }{
					// store.toml files.
					{"metadata.buildpack/store.toml", "[metadata]\n  [metadata.metadata-buildpack-store-data]\n    store-key = \"store-val\""},
					{"no.cache.buildpack/store.toml", "[metadata]\n  [metadata.no-cache-buildpack-store-data]\n    store-key = \"store-val\""},
				} {
					got := h.MustReadFile(t, filepath.Join(layerDir, data.name))
					h.AssertStringContains(t, string(got), data.want)
				}
			})
		})

		when("only cache metadata is present", func() {
			it.Before(func() {
				cacheMetaDataJSON := h.MustReadFile(t, filepath.Join("testdata", "cache_metadata.json"))
				h.AssertNil(t, json.Unmarshal(cacheMetaDataJSON, &cacheMetadata))
			})

			it("does not restore each store metadata", func() {
				err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
				h.AssertNil(t, err)
				for _, file := range []string{
					// store.toml files.
					"metadata.buildpack/store.toml",
					"no.cache.buildpack/store.toml",
				} {
					h.AssertPathDoesNotExist(t, filepath.Join(layerDir, file))
				}
			})
		})

		when("app and cache metadata are present", func() {
			it.Before(func() {
				cacheMetaDataJSON := h.MustReadFile(t, filepath.Join("testdata", "cache_metadata.json"))
				h.AssertNil(t, json.Unmarshal(cacheMetaDataJSON, &cacheMetadata))

				layerMetaDataJSON := h.MustReadFile(t, filepath.Join("testdata", "app_metadata.json"))
				h.AssertNil(t, json.Unmarshal(layerMetaDataJSON, &layersMetadata))
			})

			it("restores app and cache layer metadata", func() {
				err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
				h.AssertNil(t, err)

				unsetFlags := "[types]"
				for _, data := range []struct{ name, want string }{
					// App layers.
					{"metadata.buildpack/launch.toml", "[metadata]\n  launch-key = \"launch-value\""},
					{"metadata.buildpack/launch-build-cache.toml", "[metadata]\n  launch-build-cache-key = \"launch-build-cache-value\""},
					{"metadata.buildpack/launch-cache.toml", "[metadata]\n  launch-cache-key = \"launch-cache-value\""},
					{"no.cache.buildpack/some-layer.toml", "[metadata]\n  some-layer-key = \"some-layer-value\""},
					// Cache-image-only layers.
					{"metadata.buildpack/cache.toml", "[metadata]\n  cache-key = \"cache-value\""},
					// Cached launch layers not in app
					{"metadata.buildpack/launch-cache-not-in-app.toml", "[metadata]\n  cache-only-key = \"cache-only-value\"\n  launch-cache-key = \"cache-specific-value\""},
				} {
					got := h.MustReadFile(t, filepath.Join(layerDir, data.name))
					h.AssertStringContains(t, string(got), data.want)
					h.AssertStringDoesNotContain(t, string(got), unsetFlags) // The [types] table shouldn't exist. The build, cache and launch flags are set to false.
				}
			})

			when("platformAPI is less than 0.14", func() {
				it.Before(func() {
					layerMetadataRestorer = layer.NewDefaultMetadataRestorer(layerDir, skipLayers, &logger, api.MustParse("0.13"))
				})

				it("ignores launch-cache-not-in-app", func() {
					err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
					h.AssertNil(t, err)

					h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack/launch-cache-not-in-app.toml"))
					unsetFlags := "[types]"
					for _, data := range []struct{ name, want string }{
						// App layers.
						{"metadata.buildpack/launch.toml", "[metadata]\n  launch-key = \"launch-value\""},
						{"metadata.buildpack/launch-build-cache.toml", "[metadata]\n  launch-build-cache-key = \"launch-build-cache-value\""},
						{"metadata.buildpack/launch-cache.toml", "[metadata]\n  launch-cache-key = \"launch-cache-value\""},
						{"no.cache.buildpack/some-layer.toml", "[metadata]\n  some-layer-key = \"some-layer-value\""},
						// Cache-image-only layers.
						{"metadata.buildpack/cache.toml", "[metadata]\n  cache-key = \"cache-value\""},
					} {
						got := h.MustReadFile(t, filepath.Join(layerDir, data.name))
						h.AssertStringContains(t, string(got), data.want)
						h.AssertStringDoesNotContain(t, string(got), unsetFlags) // The [types] table shouldn't exist. The build, cache and launch flags are set to false.
					}
				})
			})

			it("restores layer metadata without the launch, build and cache flags", func() {
				buildpacks = []buildpack.GroupElement{
					{ID: "metadata.buildpack", API: api.Buildpack.Latest().String()},
					{ID: "no.cache.buildpack", API: api.Buildpack.Latest().String()},
				}

				err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
				h.AssertNil(t, err)

				unsetFlags := "[types]"
				for _, data := range []struct{ name, want string }{
					{"metadata.buildpack/launch.toml", "[metadata]\n  launch-key = \"launch-value\""},
					{"no.cache.buildpack/some-layer.toml", "[metadata]\n  some-layer-key = \"some-layer-value\""},
				} {
					got := h.MustReadFile(t, filepath.Join(layerDir, data.name))
					h.AssertStringContains(t, string(got), data.want)
					h.AssertStringDoesNotContain(t, string(got), unsetFlags) // The [types] table shouldn't exist. The build, cache and launch flags are set to false.
				}
			})

			it("does not overwrite metadata from app image", func() {
				err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
				h.AssertNil(t, err)

				for _, name := range []string{
					"metadata.buildpack/launch-build-cache.toml",
					"metadata.buildpack/launch-cache.toml",
				} {
					got := h.MustReadFile(t, filepath.Join(layerDir, name))
					avoid := "[metadata]\n  cache-only-key = \"cache-only-value\""
					if strings.Contains(string(got), avoid) {
						t.Errorf("Expected %q to not contain %q, got %q", name, avoid, got)
					}
				}
			})

			it("does not restore cache=true layers for non-selected groups", func() {
				err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
				h.AssertNil(t, err)

				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "no.group.buildpack"))
			})

			it("does not restore cache=false layer metadata", func() {
				err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
				h.AssertNil(t, err)

				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "cache-false.toml"))
				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "cache-false.sha"))
			})

			it("does not restore launch=false layer metadata", func() {
				err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
				h.AssertNil(t, err)

				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "launch-false.toml"))
				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "launch-false.sha"))
			})

			it("does not restore build=true, cache=false layer metadata", func() {
				err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
				h.AssertNil(t, err)

				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "launch-build.sha"))
			})

			it("restores escaped buildpack layer metadata", func() {
				err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
				h.AssertNil(t, err)

				path := filepath.Join(layerDir, "escaped_buildpack_id", "escaped-bp-layer.toml")
				got := h.MustReadFile(t, path)
				want := "[metadata]\n  escaped-bp-layer-key = \"escaped-bp-layer-value\""

				h.AssertStringContains(t, string(got), want)
			})

			it("restores each store metadata", func() {
				err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
				h.AssertNil(t, err)
				for _, data := range []struct{ name, want string }{
					// store.toml files.
					{"metadata.buildpack/store.toml", "[metadata]\n  [metadata.metadata-buildpack-store-data]\n    store-key = \"store-val\""},
					{"no.cache.buildpack/store.toml", "[metadata]\n  [metadata.no-cache-buildpack-store-data]\n    store-key = \"store-val\""},
				} {
					got := h.MustReadFile(t, filepath.Join(layerDir, data.name))
					h.AssertStringContains(t, string(got), data.want)
				}
			})

			when("app and cache metadata are inconsistent with each other", func() { // cache was manipulated or deleted
				it.Before(func() {
					metadata := h.MustReadFile(t, filepath.Join("testdata", "cache_inconsistent_metadata.json"))
					h.AssertNil(t, json.Unmarshal(metadata, &cacheMetadata))
				})

				when("app metadata cache=true, cache metadata cache=false", func() {
					it("treats the layer as cache=false", func() {
						err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
						h.AssertNil(t, err)

						h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "cache.toml"))
						h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "launch-build-cache.toml"))
						h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "launch-cache.toml"))
					})
				})
			})

			when("subset of buildpacks are detected", func() {
				it.Before(func() {
					buildpacks = []buildpack.GroupElement{{ID: "no.cache.buildpack", API: api.Buildpack.Latest().String()}}
				})

				it("restores layers for detected buildpacks", func() {
					err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
					h.AssertNil(t, err)

					path := filepath.Join(layerDir, "no.cache.buildpack", "some-layer.toml")
					got := h.MustReadFile(t, path)
					want := "[metadata]\n  some-layer-key = \"some-layer-value\""

					h.AssertStringContains(t, string(got), want)
				})

				it("does not restore layers for undetected buildpacks", func() {
					err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
					h.AssertNil(t, err)

					h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack"))
				})
			})

			when("there are no buildpacks are detected", func() {
				it.Before(func() {
					buildpacks = []buildpack.GroupElement{}
				})

				it("does not restore layers for any buildpacks", func() {
					err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
					h.AssertNil(t, err)

					h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack"))
				})
			})

			when("skip layers is true", func() {
				it.Before(func() {
					skipLayers = true
					layerMetadataRestorer = layer.NewDefaultMetadataRestorer(layerDir, skipLayers, &logger, api.Platform.Latest())
				})

				it("does not write buildpack layer metadata", func() {
					err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
					h.AssertNil(t, err)

					files, err := os.ReadDir(layerDir)
					h.AssertNil(t, err)
					h.AssertEq(t, len(files), 2)

					files, err = os.ReadDir(filepath.Join(layerDir, "metadata.buildpack"))
					h.AssertNil(t, err)
					// expect 1 file b/c of store.toml
					h.AssertEq(t, len(files), 1)

					files, err = os.ReadDir(filepath.Join(layerDir, "no.cache.buildpack"))
					h.AssertNil(t, err)
					// expect 1 file b/c of store.toml
					h.AssertEq(t, len(files), 1)
				})

				it("restores each store metadata", func() {
					err := layerMetadataRestorer.Restore(buildpacks, layersMetadata, cacheMetadata, layerSHAStore)
					h.AssertNil(t, err)
					for _, data := range []struct{ name, want string }{
						// store.toml files.
						{"metadata.buildpack/store.toml", "[metadata]\n  [metadata.metadata-buildpack-store-data]\n    store-key = \"store-val\""},
						{"no.cache.buildpack/store.toml", "[metadata]\n  [metadata.no-cache-buildpack-store-data]\n    store-key = \"store-val\""},
					} {
						got := h.MustReadFile(t, filepath.Join(layerDir, data.name))
						h.AssertStringContains(t, string(got), data.want)
					}
				})
			})
		})
	})
}
