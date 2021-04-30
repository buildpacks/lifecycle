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
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

func TestAnalyzer(t *testing.T) {
	for _, api := range api.Platform.Supported {
		spec.Run(t, "unit-analyzer/"+api.String(), testAnalyzerBuilder(api.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testAnalyzerBuilder(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var (
			cacheDir         string
			layersDir        string
			tmpDir           string
			skipLayers       bool
			analyzer         *lifecycle.Analyzer
			image            *fakes.Image
			metadataRestorer *lifecycle.DefaultLayerMetadataRestorer
			mockCtrl         *gomock.Controller
			testCache        lifecycle.Cache
		)

		it.Before(func() {
			var err error

			tmpDir, err = ioutil.TempDir("", "analyzer-tests")
			h.AssertNil(t, err)

			layersDir, err = ioutil.TempDir("", "lifecycle-layer-dir")
			h.AssertNil(t, err)

			cacheDir, err = ioutil.TempDir("", "some-cache-dir")
			h.AssertNil(t, err)

			testCache, err = cache.NewVolumeCache(cacheDir)
			h.AssertNil(t, err)

			image = fakes.NewImage("image-repo-name", "", local.IDIdentifier{
				ImageID: "s0m3D1g3sT",
			})

			discardLogger := log.Logger{Handler: &discard.Handler{}}

			metadataRestorer = &lifecycle.DefaultLayerMetadataRestorer{
				Logger:     &discardLogger,
				LayersDir:  layersDir,
				SkipLayers: skipLayers,
			}

			analyzer = &lifecycle.Analyzer{
				Image:    image,
				Logger:   &discardLogger,
				Platform: platform.NewPlatform(platformAPI),
				Buildpacks: []buildpack.GroupBuildpack{
					{ID: "metadata.buildpack", API: api.Buildpack.Latest().String()},
					{ID: "no.cache.buildpack", API: api.Buildpack.Latest().String()},
					{ID: "no.metadata.buildpack", API: api.Buildpack.Latest().String()},
				},
				Cache:                 testCache,
				LayerMetadataRestorer: metadataRestorer,
			}
			if testing.Verbose() {
				analyzer.Logger = cmd.DefaultLogger
				h.AssertNil(t, cmd.SetLogLevel("debug"))
			}
			mockCtrl = gomock.NewController(t)
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(tmpDir))
			h.AssertNil(t, os.RemoveAll(layersDir))
			h.AssertNil(t, os.RemoveAll(cacheDir))
			h.AssertNil(t, image.Cleanup())
			mockCtrl.Finish()
		})

		when("#Analyze", func() {
			var (
				expectedAppMetadata platform.LayersMetadata
				ref                 *testmock.MockReference
			)

			it.Before(func() {
				ref = testmock.NewMockReference(mockCtrl)
				ref.EXPECT().Name().AnyTimes()
			})

			when("image exists", func() {
				it.Before(func() {
					metadata := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "app_metadata.json"))
					h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", string(metadata)))
					h.AssertNil(t, json.Unmarshal(metadata, &expectedAppMetadata))
				})

				it("restores layer metadata without the launch, build and cache flags", func() {
					h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

					_, err := analyzer.Analyze()
					h.AssertNil(t, err)

					unsetFlags := "[types]"
					for _, data := range []struct{ name, want string }{
						{"metadata.buildpack/launch.toml", "[metadata]\n  launch-key = \"launch-value\""},
						{"no.cache.buildpack/some-layer.toml", "[metadata]\n  some-layer-key = \"some-layer-value\""},
					} {
						got := h.MustReadFile(t, filepath.Join(layersDir, data.name))
						h.AssertStringContains(t, string(got), data.want)
						h.AssertStringDoesNotContain(t, string(got), unsetFlags) // The [types] table shouldn't exist. The build, cache and launch flags are set to false.
					}
				})

				when("buildpack API < 0.6", func() {
					it.Before(func() {
						analyzer.Buildpacks = []buildpack.GroupBuildpack{
							{ID: "metadata.buildpack", API: "0.5"},
							{ID: "no.cache.buildpack", API: "0.5"},
						}
					})

					it("restores layer metadata and preserves the values of the launch, build and cache flags in top level", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						for _, data := range []struct{ name, want string }{
							{"metadata.buildpack/launch.toml", "build = false\nlaunch = true\ncache = false\n\n[metadata]\n  launch-key = \"launch-value\""},
							{"no.cache.buildpack/some-layer.toml", "build = false\nlaunch = true\ncache = false\n\n[metadata]\n  some-layer-key = \"some-layer-value\""},
						} {
							got := h.MustReadFile(t, filepath.Join(layersDir, data.name))
							h.AssertStringContains(t, string(got), data.want)
						}
					})
				})

				it("restores layer sha files", func() {
					h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

					_, err := analyzer.Analyze()
					h.AssertNil(t, err)

					for _, data := range []struct{ name, want string }{
						{"metadata.buildpack/launch.sha", "launch-sha"},
						{"no.cache.buildpack/some-layer.sha", "some-layer-sha"},
					} {
						got := h.MustReadFile(t, filepath.Join(layersDir, data.name))
						h.AssertStringContains(t, string(got), data.want)
					}
				})

				it("does not restore layer sha files", func() {
					h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) < 0, "Platform API < 0.7 restores layer metadata")

					_, err := analyzer.Analyze()
					h.AssertNil(t, err)

					for _, data := range []struct{ name, want string }{
						{"metadata.buildpack/launch.sha", ""},
						{"metadata.buildpack/launch-build-cache.sha", ""},
						{"metadata.buildpack/launch-cache.sha", ""},
						{"no.cache.buildpack/some-layer.sha", ""},
					} {
						h.AssertPathDoesNotExist(t, data.name)
					}
				})

				it("does not restore launch=false layer metadata", func() {
					h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

					_, err := analyzer.Analyze()
					h.AssertNil(t, err)

					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "launch-false.toml"))
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "launch-false.sha"))
				})

				it("does not restore build=true, cache=false layer metadata", func() {
					h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

					_, err := analyzer.Analyze()
					h.AssertNil(t, err)

					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "launch-build.sha"))
				})

				when("subset of buildpacks are detected", func() {
					it.Before(func() {
						analyzer.Buildpacks = []buildpack.GroupBuildpack{{ID: "no.cache.buildpack", API: api.Buildpack.Latest().String()}}
					})

					it("restores layers for detected buildpacks", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						path := filepath.Join(layersDir, "no.cache.buildpack", "some-layer.toml")
						got := h.MustReadFile(t, path)
						want := "[metadata]\n  some-layer-key = \"some-layer-value\""

						h.AssertStringContains(t, string(got), want)
					})

					it("does not restore layers for undetected buildpacks", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack"))
					})
				})

				it("returns the analyzed metadata", func() {
					md, err := analyzer.Analyze()
					h.AssertNil(t, err)

					h.AssertEq(t, md.Image.Reference, "s0m3D1g3sT")
					h.AssertEq(t, md.Metadata, expectedAppMetadata)
				})

				it("restores each store metadata", func() {
					h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore store metadata")

					_, err := analyzer.Analyze()
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

				when("cache exists", func() {
					it.Before(func() {
						metadata := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "cache_metadata.json"))
						var cacheMetadata platform.CacheMetadata
						h.AssertNil(t, json.Unmarshal(metadata, &cacheMetadata))
						h.AssertNil(t, testCache.SetMetadata(cacheMetadata))
						h.AssertNil(t, testCache.Commit())

						analyzer.Buildpacks = append(analyzer.Buildpacks, buildpack.GroupBuildpack{ID: "escaped/buildpack/id", API: api.Buildpack.Latest().String()})
					})

					it("restores app and cache layer metadata without the launch, build and cache flags", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
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
							got := h.MustReadFile(t, filepath.Join(layersDir, data.name))
							h.AssertStringContains(t, string(got), data.want)
							h.AssertStringDoesNotContain(t, string(got), unsetFlags) // The [types] table shouldn't exist. The build, cache and launch flags are set to false.
						}
					})

					it("restores app and cache layer sha files, prefers app sha", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						for _, data := range []struct{ name, want string }{
							{"metadata.buildpack/launch.sha", "launch-sha"},
							{"metadata.buildpack/launch-build-cache.sha", "launch-build-cache-sha"},
							{"metadata.buildpack/launch-cache.sha", "launch-cache-sha"},
							{"no.cache.buildpack/some-layer.sha", "some-layer-sha"},
							// Cache-image-only layers.
							{"metadata.buildpack/cache.sha", "cache-sha"},
						} {
							got := h.MustReadFile(t, filepath.Join(layersDir, data.name))
							h.AssertStringContains(t, string(got), data.want)
						}
					})

					it("does not overwrite metadata from app image", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						for _, name := range []string{
							"metadata.buildpack/launch-build-cache.toml",
							"metadata.buildpack/launch-cache.toml",
						} {
							got := h.MustReadFile(t, filepath.Join(layersDir, name))
							avoid := "[metadata]\n  cache-only-key = \"cache-only-value\""
							if strings.Contains(string(got), avoid) {
								t.Errorf("Expected %q to not contain %q, got %q", name, avoid, got)
							}
						}
					})

					it("does not overwrite sha from app image", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						for _, name := range []string{
							"metadata.buildpack/launch-build-cache.sha",
							"metadata.buildpack/launch-cache.sha",
						} {
							got := h.MustReadFile(t, filepath.Join(layersDir, name))
							avoid := "old-sha"
							if strings.Contains(string(got), avoid) {
								t.Errorf("Expected %q to not contain %q, got %q", name, avoid, got)
							}
						}
					})

					it("does not restore cache=true layers for non-selected groups", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "no.group.buildpack"))
					})

					it("does not restore launch=true layer metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "launch-cache-not-in-app.toml"))
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "launch-cache-not-in-app.sha"))
					})

					it("does not restore cache=false layer metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "cache-false.toml"))
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "cache-false.sha"))
					})

					it("restores escaped buildpack layer metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						path := filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer.toml")
						got := h.MustReadFile(t, path)
						want := "[metadata]\n  escaped-bp-layer-key = \"escaped-bp-layer-value\""
						unsetFlags := "[types]"

						h.AssertStringContains(t, string(got), want)
						h.AssertStringDoesNotContain(t, string(got), unsetFlags) // The [types] table shouldn't exist. The build, cache and launch flags are set to false.
					})

					when("subset of buildpacks are detected", func() {
						it.Before(func() {
							analyzer.Buildpacks = []buildpack.GroupBuildpack{{ID: "no.group.buildpack", API: api.Buildpack.Latest().String()}}
						})

						it("restores layers for detected buildpacks", func() {
							h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

							_, err := analyzer.Analyze()
							h.AssertNil(t, err)

							path := filepath.Join(layersDir, "no.group.buildpack", "some-layer.toml")
							got := h.MustReadFile(t, path)
							want := "[metadata]\n  some-layer-key = \"some-layer-value\""

							h.AssertStringContains(t, string(got), want)
						})

						it("does not restore layers for undetected buildpacks", func() {
							h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

							_, err := analyzer.Analyze()
							h.AssertNil(t, err)

							h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack"))
							h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "escaped_buildpack_id"))
						})
					})

					when("buildpack API < 0.6", func() {
						it.Before(func() {
							analyzer.Buildpacks = []buildpack.GroupBuildpack{
								{ID: "metadata.buildpack", API: "0.5"},
								{ID: "no.cache.buildpack", API: "0.5"},
							}
						})

						it("restores app and cache layer metadata and preserves the values of the launch, build and cache flags", func() {
							h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

							_, err := analyzer.Analyze()
							h.AssertNil(t, err)

							for _, data := range []struct{ name, want string }{
								// App layers.
								{"metadata.buildpack/launch.toml", "build = false\nlaunch = true\ncache = false\n\n[metadata]\n  launch-key = \"launch-value\""},
								{"metadata.buildpack/launch-build-cache.toml", "build = true\nlaunch = true\ncache = true\n\n[metadata]\n  launch-build-cache-key = \"launch-build-cache-value\""},
								{"metadata.buildpack/launch-cache.toml", "build = false\nlaunch = true\ncache = true\n\n[metadata]\n  launch-cache-key = \"launch-cache-value\""},
								{"no.cache.buildpack/some-layer.toml", "build = false\nlaunch = true\ncache = false\n\n[metadata]\n  some-layer-key = \"some-layer-value\""},
								// Cache-image-only layers.
								{"metadata.buildpack/cache.toml", "build = false\nlaunch = false\ncache = true\n\n[metadata]\n  cache-key = \"cache-value\""},
							} {
								got := h.MustReadFile(t, filepath.Join(layersDir, data.name))
								h.AssertStringContains(t, string(got), data.want)
							}
						})
					})
				})

				when("cache with inconsistent metadata exists", func() { // cache was manipulated or deleted
					it.Before(func() {
						metadata := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "cache_inconsistent_metadata.json"))
						var cacheMetadata platform.CacheMetadata
						h.AssertNil(t, json.Unmarshal(metadata, &cacheMetadata))
						h.AssertNil(t, testCache.SetMetadata(cacheMetadata))
						h.AssertNil(t, testCache.Commit())
					})

					when("app metadata cache=true, cache metadata cache=false", func() {
						it("treats the layer as cache=false", func() {
							_, err := analyzer.Analyze()
							h.AssertNil(t, err)

							h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "cache.toml"))
							h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "launch-build-cache.toml"))
							h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "launch-cache.toml"))
						})
					})
				})

				when("skip-layers is true", func() {
					it.Before(func() {
						metadataRestorer.SkipLayers = true
					})

					it("should return the analyzed metadata", func() {
						md, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertEq(t, md.Image.Reference, "s0m3D1g3sT")
						h.AssertEq(t, md.Metadata, expectedAppMetadata)
					})

					it("does not write buildpack layer metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						files, err := ioutil.ReadDir(layersDir)
						h.AssertNil(t, err)
						h.AssertEq(t, len(files), 2)

						files, err = ioutil.ReadDir(filepath.Join(layersDir, "metadata.buildpack"))
						h.AssertNil(t, err)
						//expect 1 file b/c of store.toml
						h.AssertEq(t, len(files), 1)

						files, err = ioutil.ReadDir(filepath.Join(layersDir, "no.cache.buildpack"))
						h.AssertNil(t, err)
						//expect 1 file b/c of store.toml
						h.AssertEq(t, len(files), 1)
					})

					it("restores each store metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore store metadata")

						_, err := analyzer.Analyze()
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
			})

			when("image not found", func() {
				it.Before(func() {
					h.AssertNil(t, image.Delete())
				})

				when("cache exists", func() {
					it.Before(func() {
						metadata := h.MustReadFile(t, filepath.Join("testdata", "analyzer", "cache_metadata.json"))
						var cacheMetadata platform.CacheMetadata
						h.AssertNil(t, json.Unmarshal(metadata, &cacheMetadata))
						h.AssertNil(t, testCache.SetMetadata(cacheMetadata))
						h.AssertNil(t, testCache.Commit())

						analyzer.Buildpacks = append(analyzer.Buildpacks, buildpack.GroupBuildpack{ID: "escaped/buildpack/id", API: api.Buildpack.Latest().String()})
					})

					it("restores cache=true layer metadata without the launch, build and cache flags", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						path := filepath.Join(layersDir, "metadata.buildpack/cache.toml")
						got := h.MustReadFile(t, path)
						want := "[metadata]\n  cache-key = \"cache-value\""
						unsetFlags := "[types]"

						h.AssertStringContains(t, string(got), want)
						h.AssertStringDoesNotContain(t, string(got), unsetFlags) // The [types] table shouldn't exist. The build, cache and launch flags are set to false.
					})

					it("does not restore launch=true layer metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "launch-cache.toml"))
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "launch-build-cache.toml"))
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "launch-cache-not-in-app.toml"))
					})

					it("does not restore cache=false layer metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "metadata.buildpack", "cache-false.toml"))
					})

					it("returns a nil image in the analyzed metadata", func() {
						md, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertNil(t, md.Image)
						h.AssertEq(t, md.Metadata, platform.LayersMetadata{})
					})
				})

				when("cache is empty", func() {
					it("does not restore any metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						files, err := ioutil.ReadDir(layersDir)
						h.AssertNil(t, err)
						h.AssertEq(t, len(files), 0)
					})

					it("returns a nil image in the analyzed metadata", func() {
						md, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertNil(t, md.Image)
						h.AssertEq(t, md.Metadata, platform.LayersMetadata{})
					})
				})

				when("cache is not provided", func() {
					it.Before(func() {
						analyzer.Cache = nil
					})

					it("does not restore any metadata", func() {
						h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

						_, err := analyzer.Analyze()
						h.AssertNil(t, err)

						files, err := ioutil.ReadDir(layersDir)
						h.AssertNil(t, err)
						h.AssertEq(t, len(files), 0)
					})

					it("returns a nil image in the analyzed metadata", func() {
						md, err := analyzer.Analyze()
						h.AssertNil(t, err)

						h.AssertNil(t, md.Image)
						h.AssertEq(t, md.Metadata, platform.LayersMetadata{})
					})
				})
			})

			when("image does not have metadata label", func() {
				it.Before(func() {
					h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", ""))
				})

				it("does not restore any metadata", func() {
					h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

					_, err := analyzer.Analyze()
					h.AssertNil(t, err)

					files, err := ioutil.ReadDir(layersDir)
					h.AssertNil(t, err)
					h.AssertEq(t, len(files), 0)
				})

				it("returns empty analyzed metadata", func() {
					md, err := analyzer.Analyze()
					h.AssertNil(t, err)
					h.AssertEq(t, md.Metadata, platform.LayersMetadata{})
				})
			})

			when("image has incompatible metadata", func() {
				it.Before(func() {
					h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", `{["bad", "metadata"]}`))
				})

				it("does not restore any metadata", func() {
					h.SkipIf(t, api.MustParse(platformAPI).Compare(api.MustParse("0.7")) >= 0, "Platform API >= 0.7 does not restore layer metadata")

					_, err := analyzer.Analyze()
					h.AssertNil(t, err)

					files, err := ioutil.ReadDir(layersDir)
					h.AssertNil(t, err)
					h.AssertEq(t, len(files), 0)
				})

				it("returns empty analyzed metadata", func() {
					md, err := analyzer.Analyze()
					h.AssertNil(t, err)
					h.AssertEq(t, md.Metadata, platform.LayersMetadata{})
				})
			})
		})
	}
}
