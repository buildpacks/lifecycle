package phase_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/phase/testmock"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestRestorer(t *testing.T) {
	buildpackAPIStr := api.Buildpack.Latest().String()
	for _, platformAPI := range api.Platform.Supported {
		platformAPIStr := platformAPI.String()
		spec.Run(
			t,
			"unit-restorer/buildpack-"+buildpackAPIStr+"/platform-"+platformAPIStr,
			testRestorer(buildpackAPIStr, platformAPIStr), spec.Report(report.Terminal{}),
		)
	}
}

func testRestorer(buildpackAPI, platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		when("#Restore", func() {
			var (
				cacheDir     string
				layersDir    string
				logHandler   *memory.Handler
				skipLayers   bool
				testCache    phase.Cache
				restorer     *phase.Restorer
				mockCtrl     *gomock.Controller
				sbomRestorer *testmock.MockSBOMRestorer
			)

			it.Before(func() {
				var err error
				logHandler = memory.New()
				logger := log.Logger{Handler: logHandler, Level: log.DebugLevel}

				layersDir, err = os.MkdirTemp("", "lifecycle-layer-dir")
				h.AssertNil(t, err)

				cacheDir, err = os.MkdirTemp("", "")
				h.AssertNil(t, err)

				testCache, err = cache.NewVolumeCache(cacheDir, &logger)
				h.AssertNil(t, err)

				mockCtrl = gomock.NewController(t)
				sbomRestorer = testmock.NewMockSBOMRestorer(mockCtrl)
				if api.MustParse(platformAPI).AtLeast("0.8") {
					sbomRestorer.EXPECT().RestoreToBuildpackLayers(gomock.Any()).AnyTimes()
				}

				restorer = &phase.Restorer{
					LayersDir: layersDir,
					Logger:    &logger,
					Buildpacks: []buildpack.GroupElement{
						{ID: "buildpack.id", API: buildpackAPI},
						{ID: "escaped/buildpack/id", API: buildpackAPI},
					},
					LayerMetadataRestorer: layer.NewDefaultMetadataRestorer(layersDir, skipLayers, &logger, api.Platform.Latest()),
					SBOMRestorer:          sbomRestorer,
					PlatformAPI:           api.MustParse(platformAPI),
				}
			})

			it.After(func() {
				h.AssertNil(t, os.RemoveAll(layersDir))
				h.AssertNil(t, os.RemoveAll(cacheDir))
				mockCtrl.Finish()
			})

			when("there is no cache", func() {
				when("there is a cache=true layer", func() {
					it.Before(func() {
						var meta, sha string
						h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-true", meta, sha))
						h.AssertNil(t, restorer.Restore(nil))
					})

					it("does not restore layer data", func() {
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-true"))
					})
				})

				when("there is a cache=false layer", func() {
					it.Before(func() {
						var meta, sha string
						h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-false", meta, sha))
						h.AssertNil(t, restorer.Restore(testCache))
					})

					it("keeps metadata file", func() {
						h.AssertPathExists(t, filepath.Join(layersDir, "buildpack.id", "cache-false.toml"))
					})

					it("does not restore layer data", func() {
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-false"))
					})
				})
			})

			when("there is an empty cache", func() {
				when("there is a cache=true layer", func() {
					it.Before(func() {
						var meta, sha string
						h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-true", meta, sha))
						h.AssertNil(t, restorer.Restore(testCache))
					})

					it("does not restore layer data", func() {
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-true"))
					})
				})

				when("there is a cache=false layer", func() {
					it.Before(func() {
						var meta, sha string
						h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-false", meta, sha))
						h.AssertNil(t, restorer.Restore(testCache))
					})

					it("keeps metadata file", func() {
						h.AssertPathExists(t, filepath.Join(layersDir, "buildpack.id", "cache-false.toml"))
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

					tarTempDir, err = os.MkdirTemp("", "restorer-test-temp-layer")
					h.AssertNil(t, err)

					lf := layers.Factory{
						ArtifactsDir: tarTempDir,
						Logger:       nil,
					}
					layer, err := lf.DirLayer("buildpack.id:cache-only", filepath.Join(layersDir, "buildpack.id", "cache-only"), "")
					h.AssertNil(t, err)
					cacheOnlyLayerSHA = layer.Digest
					h.AssertNil(t, testCache.AddLayerFile(layer.TarPath, layer.Digest))

					layer, err = lf.DirLayer("buildpack.id:cache-false", filepath.Join(layersDir, "buildpack.id", "cache-false"), "")
					h.AssertNil(t, err)
					cacheFalseLayerSHA = layer.Digest
					h.AssertNil(t, testCache.AddLayerFile(layer.TarPath, layer.Digest))

					layer, err = lf.DirLayer("buildpack.id:cache-launch", filepath.Join(layersDir, "buildpack.id", "cache-launch"), "")
					h.AssertNil(t, err)
					cacheLaunchLayerSHA = layer.Digest
					h.AssertNil(t, testCache.AddLayerFile(layer.TarPath, layer.Digest))

					layer, err = lf.DirLayer("nogroup.buildpack.id:some-layer", filepath.Join(layersDir, "nogroup.buildpack.id", "some-layer"), "")
					h.AssertNil(t, err)
					noGroupLayerSHA = layer.Digest
					h.AssertNil(t, testCache.AddLayerFile(layer.TarPath, layer.Digest))

					layer, err = lf.DirLayer("escaped/buildpack/id.id:escaped-bp-layer", filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer"), "")
					h.AssertNil(t, err)
					escapedLayerSHA = layer.Digest
					h.AssertNil(t, testCache.AddLayerFile(layer.TarPath, layer.Digest))

					h.AssertNil(t, testCache.Commit())
					h.AssertNil(t, os.RemoveAll(layersDir))
					h.AssertNil(t, os.Mkdir(layersDir, 0777))

					contents := buildMetadata(cacheFalseLayerSHA, cacheLaunchLayerSHA, cacheOnlyLayerSHA, noGroupLayerSHA, escapedLayerSHA)

					err = os.WriteFile(
						filepath.Join(cacheDir, "committed", "io.buildpacks.lifecycle.cache.metadata"),
						[]byte(contents),
						0600,
					)
					h.AssertNil(t, err)
				})

				it.After(func() {
					h.AssertNil(t, os.RemoveAll(tarTempDir))
				})

				when("there is a cache=true layer", func() {
					var meta string

					it.Before(func() {
						meta += "[metadata]\n  cache-only-key = \"cache-only-val\"\n"
						var sha string
						h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-only", meta, sha))
						h.AssertNil(t, restorer.Restore(testCache))
					})

					it("keeps layer metadata", func() {
						got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-only.toml"))
						h.AssertEq(t, string(got), meta)
					})

					it("restores data", func() {
						got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-only", "file-from-cache-only-layer"))
						want := "echo text from cache-only layer\n"
						h.AssertEq(t, string(got), want)
					})
				})

				when("there is a cache=false layer", func() {
					var meta string
					it.Before(func() {
						meta = "[metadata]\n  cache-false-key = \"cache-false-val\""
						var sha string
						h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-false", meta, sha))
						h.AssertNil(t, restorer.Restore(testCache))
					})

					it("keeps layer metadata", func() {
						got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-false.toml"))
						h.AssertEq(t, string(got), meta)
					})

					it("does not restore data", func() {
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-false"))
					})
				})

				when("there is a cache=true layer with wrong sha", func() {
					var otherSHA string
					it.Before(func() {
						otherSHA = "some-made-up-sha"
						var meta, layerSha string
						h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-launch", meta, layerSha))

						appMetaContents := []byte(fmt.Sprintf(`{
   "buildpacks": [
       {
           "key": "buildpack.id",
           "layers": {
               "cache-launch": {
                   "data": {
                       "cache-launch-key": "cache-launch-val"
                   },
                   "cache": true,
                   "launch": true,
                   "sha": "%s"
               }
           }
       }
   ]
}
`, otherSHA))

						h.AssertNil(t, json.Unmarshal(appMetaContents, &restorer.LayersMetadata))

						h.AssertNil(t, restorer.Restore(testCache))
					})

					it("removes metadata file", func() {
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-launch.toml"))
					})

					it("does not restore layer data", func() {
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-launch"))
						expected := "Removing \"buildpack.id:cache-launch\", wrong sha"
						assertLogEntry(t, logHandler, expected)
						expected = fmt.Sprintf("Layer sha: %q", otherSHA)
						assertLogEntry(t, logHandler, expected)
					})
				})

				when("there is a cache-only layer referenced in metadata that does not exist", func() {
					var nonExistentCacheLaunchLayerSHA string

					it.Before(func() {
						nonExistentCacheLaunchLayerSHA = "some-made-up-sha"
						contents := buildMetadata(cacheFalseLayerSHA, nonExistentCacheLaunchLayerSHA, cacheOnlyLayerSHA, noGroupLayerSHA, escapedLayerSHA)

						err := os.WriteFile(
							filepath.Join(cacheDir, "committed", "io.buildpacks.lifecycle.cache.metadata"),
							[]byte(contents),
							0600,
						)
						h.AssertNil(t, err)

						err = restorer.Restore(testCache)
						h.AssertNil(t, err)
					})

					it("restores expected cache-only layer", func() {
						got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-only", "file-from-cache-only-layer"))
						want := "echo text from cache-only layer\n"
						h.AssertEq(t, string(got), want)
					})

					it("keeps expected layer metadata", func() {
						got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-only.toml"))
						h.AssertEq(t, string(got), "[metadata]\n  cache-only-key = \"cache-only-val\"\n")
					})

					it("skips restoring non-existent cache-launch layer", func() {
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-launch"))
					})
				})

				when("there is a cache=true layer not in cache", func() {
					it.Before(func() {
						var meta, sha string
						h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-layer-not-in-cache", meta, sha))
						h.AssertNil(t, restorer.Restore(testCache))
					})

					it("does not restore layer data", func() {
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "buildpack.id", "cache-layer-not-in-cache"))
					})
				})

				when("there is a cache=true escaped layer", func() {
					var meta string
					it.Before(func() {
						meta += "[metadata]\n  escaped-bp-key = \"escaped-bp-val\"\n"
						var sha string
						h.AssertNil(t, writeLayer(layersDir, "escaped_buildpack_id", "escaped-bp-layer", meta, sha))
						h.AssertNil(t, restorer.Restore(testCache))
					})

					it("keeps layer metadata", func() {
						got := h.MustReadFile(t, filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer.toml"))
						h.AssertEq(t, string(got), meta)
					})

					it("restores data", func() {
						got := h.MustReadFile(t, filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer", "file-from-escaped-bp"))
						want := "echo text from escaped bp layer\n"
						h.AssertEq(t, string(got), want)
					})
				})

				when("there is a cache=true layer in cache but not in group", func() {
					it.Before(func() {
						var meta, sha string
						h.AssertNil(t, writeLayer(layersDir, "nogroup.buildpack.id", "some-layer", meta, sha))
						h.AssertNil(t, restorer.Restore(testCache))
					})

					it("does not restore layer data", func() {
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "nogroup.buildpack.id", "some-layer"))
					})

					when("the buildpack is detected", func() {
						it.Before(func() {
							restorer.Buildpacks = []buildpack.GroupElement{{ID: "nogroup.buildpack.id", API: buildpackAPI}}
							h.AssertNil(t, restorer.Restore(testCache))
						})

						it("keeps metadata file", func() {
							h.AssertPathExists(t, filepath.Join(layersDir, "nogroup.buildpack.id", "some-layer.toml"))
						})

						it("restores data", func() {
							got := h.MustReadFile(t, filepath.Join(layersDir, "nogroup.buildpack.id", "some-layer", "file-from-some-layer"))
							want := "echo text from some layer\n"
							h.AssertEq(t, string(got), want)
						})
					})
				})

				when("there are multiple cache=true layers", func() {
					var cacheOnlyMeta, cacheLaunchMeta, escapedMeta string

					it.Before(func() {
						var typesMeta, cacheOnlySha, cacheLaunchSha, escapedSha string

						cacheOnlyMeta = typesMeta + "[metadata]\n  cache-only-key = \"cache-only-val\"\n"
						h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-only", cacheOnlyMeta, cacheOnlySha))

						escapedMeta = typesMeta + "[metadata]\n  escaped-bp-key = \"escaped-bp-val\"\n"
						h.AssertNil(t, writeLayer(layersDir, "escaped_buildpack_id", "escaped-bp-layer", escapedMeta, escapedSha))

						cacheLaunchMeta = typesMeta + "[metadata]\n  cache-launch-key = \"cache-launch-val\"\n"
						h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-launch", cacheLaunchMeta, cacheLaunchSha))

						appMetaContents := []byte(fmt.Sprintf(`{
   "buildpacks": [
       {
           "key": "buildpack.id",
           "layers": {
               "cache-launch": {
                   "data": {
                       "cache-launch-key": "cache-launch-val"
                   },
                   "cache": true,
                   "launch": true,
                   "sha": "%s"
               }
           }
       }
   ]
}
`, cacheLaunchLayerSHA))

						h.AssertNil(t, json.Unmarshal(appMetaContents, &restorer.LayersMetadata))

						h.AssertNil(t, restorer.Restore(testCache))
					})

					it("keeps layer metadata for all layers", func() {
						got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-only.toml"))
						h.AssertEq(t, string(got), cacheOnlyMeta)
						got = h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-launch.toml"))
						h.AssertEq(t, string(got), cacheLaunchMeta)
						got = h.MustReadFile(t, filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer.toml"))
						h.AssertEq(t, string(got), escapedMeta)
					})

					it("restores data for all layers", func() {
						got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-only", "file-from-cache-only-layer"))
						want := "echo text from cache-only layer\n"
						h.AssertEq(t, string(got), want)
						got = h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-launch", "file-from-cache-launch-layer"))
						want = "echo text from cache launch layer\n"
						h.AssertEq(t, string(got), want)
						got = h.MustReadFile(t, filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer", "file-from-escaped-bp"))
						want = "echo text from escaped bp layer\n"
						h.AssertEq(t, string(got), want)
					})
				})
			})

			when("there is a cache with BOM information", func() {
				var (
					tmpDir string
				)

				it.Before(func() {
					h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.8"), "Platform API < 0.8 does not restore SBOM")

					tmpDir, err := os.MkdirTemp("", "")
					h.AssertNil(t, err)
					h.Mkfile(t, "some-data", filepath.Join(tmpDir, "some.tar"))
					h.AssertNil(t, testCache.AddLayerFile(filepath.Join(tmpDir, "some.tar"), "some-digest"))
					h.AssertNil(t, testCache.SetMetadata(platform.CacheMetadata{BOM: files.LayerMetadata{SHA: "some-digest"}}))
					h.AssertNil(t, testCache.Commit())
				})

				it.After(func() {
					h.AssertNil(t, os.RemoveAll(tmpDir))
				})

				it("restores the SBOM layer from the cache", func() {
					sbomRestorer.EXPECT().RestoreFromCache(testCache, "some-digest")
					err := restorer.Restore(testCache)
					h.AssertNil(t, err)
				})
			})

			when("there is no app image metadata", func() {
				it.Before(func() {
					restorer.LayersMetadata = files.LayersMetadata{}
				})

				it("analyzes with no layer metadata", func() {
					err := restorer.Restore(testCache)
					h.AssertNil(t, err)
				})
			})
		})
	}
}

func writeLayer(layersDir, buildpack, name, metadata, sha string) error {
	buildpackDir := filepath.Join(layersDir, buildpack)
	if err := os.MkdirAll(buildpackDir, 0755); err != nil {
		return errors.Wrapf(err, "creating buildpack layer directory")
	}
	metadataPath := filepath.Join(buildpackDir, name+".toml")
	if err := os.WriteFile(metadataPath, []byte(metadata), 0600); err != nil {
		return errors.Wrapf(err, "writing metadata file")
	}
	if sha != "" { // don't write a sha file when sha is an empty string
		shaPath := filepath.Join(buildpackDir, name+".sha")
		if err := os.WriteFile(shaPath, []byte(sha), 0600); err != nil {
			return errors.Wrapf(err, "writing sha file")
		}
	}
	return nil
}

func TestWriteLayer(t *testing.T) {
	layersDir, err := os.MkdirTemp("", "test-write-layer")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(layersDir)

	h.AssertNil(t, writeLayer(layersDir, "test-buildpack", "test-layer", "test-metadata", "test-sha"))

	got := h.MustReadFile(t, filepath.Join(layersDir, "test-buildpack", "test-layer.toml"))
	want := "test-metadata"
	h.AssertEq(t, string(got), want)

	got = h.MustReadFile(t, filepath.Join(layersDir, "test-buildpack", "test-layer.sha"))
	want = "test-sha"
	h.AssertEq(t, string(got), want)

	h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "test-buildpack", "test-layer"))
}

func buildMetadata(cacheFalseLayerSHA string, cacheLaunchLayerSHA string, cacheOnlyLayerSHA string, noGroupLayerSHA string, escapedLayerSHA string) string {
	return fmt.Sprintf(`{
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
                        "cache-only-key": "cache-only-val"
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
                    "data": {
                        "escaped-bp-key": "escaped-bp-val"
                    },
                    "sha": "%s"
                }
            }
        }
    ]
}
`, cacheFalseLayerSHA, cacheLaunchLayerSHA, cacheOnlyLayerSHA, noGroupLayerSHA, escapedLayerSHA)
}
