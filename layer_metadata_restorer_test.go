package lifecycle_test

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestLayerMetadataRestorer(t *testing.T) {
	spec.Run(t, "LayerMetadataRestorer", testLayerMetadataRestorer, spec.Report(report.Terminal{}))
}

func testLayerMetadataRestorer(t *testing.T, when spec.G, it spec.S) {
	var (
		layerDir       string
		layerAnalyzer  lifecycle.LayerMetadataRestorer
		layersMetadata platform.LayersMetadata
		cacheMetadata  platform.CacheMetadata
		buildpacks     []buildpack.GroupBuildpack
		skipLayers     bool
	)

	it.Before(func() {
		var err error

		layerDir, err = ioutil.TempDir("", "lifecycle-layer-dir")
		h.AssertNil(t, err)
		logger := &log.Logger{Handler: &discard.Handler{}}
		layerAnalyzer = lifecycle.NewLayerMetadataRestorer(logger, layerDir, skipLayers)
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(layerDir))
	})

	when("#Restore", func() {
		it.Before(func() {
			buildpacks = []buildpack.GroupBuildpack{
				{ID: "metadata.buildpack", API: api.Buildpack.Latest().String()},
				{ID: "no.cache.buildpack", API: api.Buildpack.Latest().String()},
				{ID: "escaped/buildpack/id", API: api.Buildpack.Latest().String()},
			}
		})

		when("app and cache metadata are not present", func() {
			it("does not restore any metadata", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
				h.AssertNil(t, err)

				files, err := ioutil.ReadDir(layerDir)
				h.AssertNil(t, err)
				h.AssertEq(t, len(files), 0)
			})
		})

		when("only app metadata is present", func() {
			it.Before(func() {
				layerMetaDataJSON := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "app_metadata.json"))
				h.AssertNil(t, json.Unmarshal(layerMetaDataJSON, &layersMetadata))
			})

			it("restores each store metadata", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
				h.AssertNil(t, err)

				files, err := ioutil.ReadDir(layerDir)
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
				cacheMetaDataJSON := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "cache_metadata.json"))
				h.AssertNil(t, json.Unmarshal(cacheMetaDataJSON, &cacheMetadata))
			})

			it("does not restore each store metadata", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
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
				cacheMetaDataJSON := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "cache_metadata.json"))
				h.AssertNil(t, json.Unmarshal(cacheMetaDataJSON, &cacheMetadata))

				layerMetaDataJSON := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "app_metadata.json"))
				h.AssertNil(t, json.Unmarshal(layerMetaDataJSON, &layersMetadata))
			})

			it("restores app and cache layer metadata", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
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
				} {
					got := h.MustReadFile(t, filepath.Join(layerDir, data.name))
					h.AssertStringContains(t, string(got), data.want)
					h.AssertStringDoesNotContain(t, string(got), unsetFlags) // The [types] table shouldn't exist. The build, cache and launch flags are set to false.
				}
			})

			it("restores layer metadata and preserves the values of the launch, build and cache flags in top level", func() {
				buildpacks = []buildpack.GroupBuildpack{
					{ID: "metadata.buildpack", API: "0.5"},
					{ID: "no.cache.buildpack", API: "0.5"},
				}

				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
				h.AssertNil(t, err)

				for _, data := range []struct{ name, want string }{
					{"metadata.buildpack/launch.toml", "build = false\nlaunch = true\ncache = false\n\n[metadata]\n  launch-key = \"launch-value\""},
					{"no.cache.buildpack/some-layer.toml", "build = false\nlaunch = true\ncache = false\n\n[metadata]\n  some-layer-key = \"some-layer-value\""},
				} {
					got := h.MustReadFile(t, filepath.Join(layerDir, data.name))
					h.AssertStringContains(t, string(got), data.want)
				}
			})

			it("restores app and cache layer sha files, prefers app sha", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
				h.AssertNil(t, err)

				for _, data := range []struct{ name, want string }{
					{"metadata.buildpack/launch.sha", "launch-sha"},
					{"metadata.buildpack/launch-build-cache.sha", "launch-build-cache-sha"},
					{"metadata.buildpack/launch-cache.sha", "launch-cache-sha"},
					{"no.cache.buildpack/some-layer.sha", "some-layer-sha"},
					// Cache-image-only layers.
					{"metadata.buildpack/cache.sha", "cache-sha"},
				} {
					got := h.MustReadFile(t, filepath.Join(layerDir, data.name))
					h.AssertStringContains(t, string(got), data.want)
				}
			})

			when("cache with inconsistent metadata exists", func() { // cache was manipulated or deleted
				it.Before(func() {
					metadata := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "cache_inconsistent_metadata.json"))
					h.AssertNil(t, json.Unmarshal(metadata, &cacheMetadata))
				})

				when("app metadata cache=true, cache metadata cache=false", func() {
					it("treats the layer as cache=false", func() {
						err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
						h.AssertNil(t, err)

						h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "cache.toml"))
						h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "launch-build-cache.toml"))
						h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "launch-cache.toml"))
					})
				})
			})

			it("does not overwrite metadata from app image", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
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

			it("does not overwrite sha from app image", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
				h.AssertNil(t, err)

				for _, name := range []string{
					"metadata.buildpack/launch-build-cache.sha",
					"metadata.buildpack/launch-cache.sha",
				} {
					got := h.MustReadFile(t, filepath.Join(layerDir, name))
					avoid := "old-sha"
					if strings.Contains(string(got), avoid) {
						t.Errorf("Expected %q to not contain %q, got %q", name, avoid, got)
					}
				}
			})

			it("does not restore cache=true layers for non-selected groups", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
				h.AssertNil(t, err)

				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "no.group.buildpack"))
			})

			it("does not restore launch=true layer metadata", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
				h.AssertNil(t, err)

				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "launch-cache-not-in-app.toml"))
				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "launch-cache-not-in-app.sha"))
			})

			it("does not restore cache=false layer metadata", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
				h.AssertNil(t, err)

				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "cache-false.toml"))
				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "cache-false.sha"))
			})

			it("does not restore launch=false layer metadata", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
				h.AssertNil(t, err)

				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "launch-false.toml"))
				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "launch-false.sha"))
			})

			it("does not restore build=true, cache=false layer metadata", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
				h.AssertNil(t, err)

				h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack", "launch-build.sha"))
			})

			it("restores escaped buildpack layer metadata", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
				h.AssertNil(t, err)

				path := filepath.Join(layerDir, "escaped_buildpack_id", "escaped-bp-layer.toml")
				got := h.MustReadFile(t, path)
				want := "[metadata]\n  escaped-bp-layer-key = \"escaped-bp-layer-value\""

				h.AssertStringContains(t, string(got), want)
			})

			it("restores each store metadata", func() {
				err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
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

			when("subset of buildpacks are detected", func() {
				it.Before(func() {
					buildpacks = []buildpack.GroupBuildpack{{ID: "no.cache.buildpack", API: api.Buildpack.Latest().String()}}
				})

				it("restores layers for detected buildpacks", func() {
					err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
					h.AssertNil(t, err)

					path := filepath.Join(layerDir, "no.cache.buildpack", "some-layer.toml")
					got := h.MustReadFile(t, path)
					want := "[metadata]\n  some-layer-key = \"some-layer-value\""

					h.AssertStringContains(t, string(got), want)
				})

				it("does not restore layers for undetected buildpacks", func() {
					err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
					h.AssertNil(t, err)

					h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack"))
				})
			})

			when("there are no buildpacks are detected", func() {
				it.Before(func() {
					buildpacks = []buildpack.GroupBuildpack{}
				})

				it("does not restore layers for any buildpacks", func() {
					err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
					h.AssertNil(t, err)

					h.AssertPathDoesNotExist(t, filepath.Join(layerDir, "metadata.buildpack"))
				})
			})

			when("SkipLayers is true", func() {
				skipLayers = true

				it("does not write buildpack layer metadata", func() {
					err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
					h.AssertNil(t, err)

					files, err := ioutil.ReadDir(layerDir)
					h.AssertNil(t, err)
					h.AssertEq(t, len(files), 2)

					files, err = ioutil.ReadDir(filepath.Join(layerDir, "metadata.buildpack"))
					h.AssertNil(t, err)
					//expect 1 file b/c of store.toml
					h.AssertEq(t, len(files), 1)

					files, err = ioutil.ReadDir(filepath.Join(layerDir, "no.cache.buildpack"))
					h.AssertNil(t, err)
					//expect 1 file b/c of store.toml
					h.AssertEq(t, len(files), 1)
				})

				it("restores each store metadata", func() {
					err := layerAnalyzer.Restore(buildpacks, layersMetadata, cacheMetadata)
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
