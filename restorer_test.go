package lifecycle_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/archive"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestRestorer(t *testing.T) {
	spec.Run(t, "Restorer", testRestorer, spec.Report(report.Terminal{}))
}

func testRestorer(t *testing.T, when spec.G, it spec.S) {
	when("#Restore", func() {
		var (
			layersDir string
			cacheDir  string
			testCache lifecycle.Cache
			restorer  *lifecycle.Restorer
		)

		it.Before(func() {
			var err error

			layersDir, err = ioutil.TempDir("", "lifecycle-layer-dir")
			h.AssertNil(t, err)

			cacheDir, err = ioutil.TempDir("", "")
			h.AssertNil(t, err)

			testCache, err = cache.NewVolumeCache(cacheDir)
			h.AssertNil(t, err)

			restorer = &lifecycle.Restorer{
				LayersDir: layersDir,
				Buildpacks: []lifecycle.Buildpack{
					{ID: "buildpack.id"},
					{ID: "escaped/buildpack/id"},
				},
				Logger: &log.Logger{Handler: &discard.Handler{}},
			}
			if testing.Verbose() {
				restorer.Logger = cmd.Logger
				cmd.SetLogLevel("debug")
			}
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(layersDir))
			h.AssertNil(t, os.RemoveAll(cacheDir))
		})

		when("there is an no cache", func() {
			when("there is a cache=true layer", func() {
				it.Before(func() {
					meta := "cache=true"
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
					meta := "cache=false"
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
					meta := "cache=true"
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
					meta := "cache=false"
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

				cacheOnlyLayerSHA = addLayerFromPath(
					t,
					tarTempDir,
					filepath.Join(layersDir, "buildpack.id", "cache-only"),
					testCache,
				)

				cacheFalseLayerSHA = addLayerFromPath(
					t,
					tarTempDir,
					filepath.Join(layersDir, "buildpack.id", "cache-false"),
					testCache,
				)

				cacheLaunchLayerSHA = addLayerFromPath(
					t,
					tarTempDir,
					filepath.Join(layersDir, "buildpack.id", "cache-launch"),
					testCache,
				)

				noGroupLayerSHA = addLayerFromPath(
					t,
					tarTempDir,
					filepath.Join(layersDir, "nogroup.buildpack.id", "some-layer"),
					testCache,
				)

				escapedLayerSHA = addLayerFromPath(
					t,
					tarTempDir,
					filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer"),
					testCache,
				)

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
					0666,
				)
				h.AssertNil(t, err)
			})

			it.After(func() {
				os.RemoveAll(tarTempDir)
			})

			when("there is a cache=true layer", func() {
				var meta string
				it.Before(func() {
					meta = `cache=true
[metadata]
  cache-only-key = "cache-only-val"`
					h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-only", meta, cacheOnlyLayerSHA))
					h.AssertNil(t, restorer.Restore(testCache))
				})

				it("keeps layer metadatata", func() {
					got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-only.toml"))
					h.AssertEq(t, string(got), meta)
				})
				it("keeps layer sha", func() {
					got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-only.sha"))
					h.AssertEq(t, string(got), cacheOnlyLayerSHA)
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
					meta = `cache=false
[metadata]
  cache-false-key = "cache-false-val"`
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
					meta := "cache=true"
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
					meta := "cache=true"
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

			when("there is a cache=true escaped layer", func() {
				var meta string
				it.Before(func() {
					meta = `cache=true
[metadata]
  escaped-bp-key = "escaped-bp-val"`
					h.AssertNil(t, writeLayer(layersDir, "escaped_buildpack_id", "escaped-bp-layer", meta, escapedLayerSHA))
					h.AssertNil(t, restorer.Restore(testCache))
				})

				it("keeps layer metadatata", func() {
					got := h.MustReadFile(t, filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer.toml"))
					h.AssertEq(t, string(got), meta)
				})
				it("keeps layer sha", func() {
					got := h.MustReadFile(t, filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer.sha"))
					h.AssertEq(t, string(got), escapedLayerSHA)
				})
				it("restores data", func() {
					got := h.MustReadFile(t, filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer", "file-from-escaped-bp"))
					want := "echo text from escaped bp layer\n"
					h.AssertEq(t, string(got), want)
				})
			})

			when("there is a cache=true layer in cache but not in group", func() {
				it.Before(func() {
					meta := "cache=true"
					h.AssertNil(t, writeLayer(layersDir, "nogroup.buildpack.id", "some-layer", meta, noGroupLayerSHA))
					h.AssertNil(t, restorer.Restore(testCache))
				})
				it("does not restore layer data", func() {
					h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "nogroup.buildpack.id", "some-layer"))
				})

				when("the buildpack is detected", func() {
					it.Before(func() {
						restorer.Buildpacks = []lifecycle.Buildpack{{ID: "nogroup.buildpack.id"}}
						h.AssertNil(t, restorer.Restore(testCache))
					})

					it("keeps metadata and sha file", func() {
						h.AssertPathExists(t, filepath.Join(layersDir, "nogroup.buildpack.id", "some-layer.toml"))
						h.AssertPathExists(t, filepath.Join(layersDir, "nogroup.buildpack.id", "some-layer.sha"))
					})
					it("restores data", func() {
						got := h.MustReadFile(t, filepath.Join(layersDir, "nogroup.buildpack.id", "some-layer", "file-from-some-layer"))
						want := "echo text from some layer\n"
						h.AssertEq(t, string(got), want)
					})
				})
			})

			when("there are multiple cache=true layers", func() {
				it.Before(func() {
					meta := "cache=true"
					h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-only", meta, cacheOnlyLayerSHA))
					meta = "cache=true\nlaunch=true"
					h.AssertNil(t, writeLayer(layersDir, "buildpack.id", "cache-launch", meta, cacheLaunchLayerSHA))
					meta = "cache=true"
					h.AssertNil(t, writeLayer(layersDir, "escaped_buildpack_id", "escaped-bp-layer", meta, escapedLayerSHA))

					h.AssertNil(t, restorer.Restore(testCache))
				})

				it("keeps layer metadatata for all layers", func() {
					got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-only.toml"))
					h.AssertEq(t, string(got), "cache=true")
					got = h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-launch.toml"))
					h.AssertEq(t, string(got), "cache=true\nlaunch=true")
					got = h.MustReadFile(t, filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer.toml"))
					h.AssertEq(t, string(got), "cache=true")
				})
				it("keeps layer sha for all layers", func() {
					got := h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-only.sha"))
					h.AssertEq(t, string(got), cacheOnlyLayerSHA)
					got = h.MustReadFile(t, filepath.Join(layersDir, "buildpack.id", "cache-launch.sha"))
					h.AssertEq(t, string(got), cacheLaunchLayerSHA)
					got = h.MustReadFile(t, filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer.sha"))
					h.AssertEq(t, string(got), escapedLayerSHA)
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
	})
}

func addLayerFromPath(t *testing.T, tarTempDir, layerPath string, c lifecycle.Cache) string {
	t.Helper()
	tarPath := filepath.Join(tarTempDir, h.RandString(10)+".tar")
	sha, err := archive.WriteTarFile(layerPath, tarPath, 0, 0, archive.DefaultTarWriterFactory())
	h.AssertNil(t, err)
	h.AssertNil(t, c.AddLayerFile(tarPath, sha))
	return sha
}

func writeLayer(layersDir, buildpack, name, metadata, sha string) error {
	buildpackDir := filepath.Join(layersDir, buildpack)
	if err := os.MkdirAll(buildpackDir, 0755); err != nil {
		return errors.Wrapf(err, "creating buildpack layer directory")
	}
	metadataPath := filepath.Join(buildpackDir, name+".toml")
	if err := ioutil.WriteFile(metadataPath, []byte(metadata), 0755); err != nil {
		return errors.Wrapf(err, "writing metadata file")
	}
	shaPath := filepath.Join(buildpackDir, name+".sha")
	if err := ioutil.WriteFile(shaPath, []byte(sha), 0755); err != nil {
		return errors.Wrapf(err, "writing sha file")
	}
	return nil
}

func TestWriteLayer(t *testing.T) {
	layersDir, err := ioutil.TempDir("", "test-write-layer")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(layersDir)

	writeLayer(layersDir, "test-buildpack", "test-layer", "test-metadata", "test-sha")

	got := h.MustReadFile(t, filepath.Join(layersDir, "test-buildpack", "test-layer.toml"))
	want := "test-metadata"
	h.AssertEq(t, string(got), want)

	got = h.MustReadFile(t, filepath.Join(layersDir, "test-buildpack", "test-layer.sha"))
	want = "test-sha"
	h.AssertEq(t, string(got), want)

	h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "test-buildpack", "test-layer"))
}
