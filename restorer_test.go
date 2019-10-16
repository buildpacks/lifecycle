package lifecycle_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/archive"
	"github.com/buildpack/lifecycle/cache"
	h "github.com/buildpack/lifecycle/testhelpers"
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
				UID:    1234,
				GID:    4321,
			}
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(layersDir))
			h.AssertNil(t, os.RemoveAll(cacheDir))
		})

		when("there is no previous cache", func() {
			it("does nothing", func() {
				h.AssertNil(t, restorer.Restore(testCache))
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
				        "cache-only": {
				          "data": {
				            "cache-only-key": "cache-only-val"
				          },
				          "cache": true,
				          "sha": "%s"
				        },
                       "cache-false": {
				          "data": {
				            "cache-false-key": "cache-false-val"
				          },
				          "cache": false,
				          "sha": "%s"
				        },
						"cache-launch": {
				          "data": {
				            "cache-launch-key": "cache-launch-val"
				          },
				          "cache": true,
						  "launch": true,
				          "sha": "%s"
				        }
				      }
				    }, {
					  "key": "nogroup.buildpack.id",
				      "layers": {
				        "some-layer": {
				          "data": {
				            "no-group-key": "no-group-val"
				          },
				          "cache": true,
				          "sha": "%s"
				        }
				      }
                   }, {
					  "key": "escaped/buildpack/id",
				      "layers": {
				        "escaped-bp-layer": {
				          "data": {
				            "escaped-bp-key": "escaped-bp-val"
				          },
				          "cache": true,
				          "sha": "%s"
				        }
				      }
					}
				  ]
				}`, cacheOnlyLayerSHA, cacheFalseLayerSHA, cacheLaunchLayerSHA, noGroupLayerSHA, escapedLayerSHA)

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

			it("restores cached layers", func() {
				h.AssertNil(t, restorer.Restore(testCache))
				expectedMetadata := `[metadata]
  cache-only-key = "cache-only-val"`
				if txt, err := ioutil.ReadFile(filepath.Join(layersDir, "buildpack.id", "cache-only.toml")); err != nil {
					t.Fatalf("failed to read cache-only.toml: %s", err)
				} else if !strings.Contains(string(txt), expectedMetadata) {
					t.Fatalf(`Error: expected '%s' to contain '%s'`, txt, expectedMetadata)
				}

				expectedText := "echo text from cache-only layer"
				if txt, err := ioutil.ReadFile(filepath.Join(layersDir, "buildpack.id", "cache-only", "file-from-cache-only-layer")); err != nil {
					t.Fatalf("failed to read file-from-cache-only-layer: %s", err)
				} else if !strings.Contains(string(txt), expectedText) {
					t.Fatalf(`Error: expected '%s' to contain '%s'`, txt, expectedText)
				}
			})

			it("write a .sha file for launch layers", func() {
				h.AssertNil(t, restorer.Restore(testCache))
				expectedMetadata := `[metadata]
  cache-launch-key = "cache-launch-val"`
				if txt, err := ioutil.ReadFile(filepath.Join(layersDir, "buildpack.id", "cache-launch.toml")); err != nil {
					t.Fatalf("failed to read cache-launch.toml: %s", err)
				} else if !strings.Contains(string(txt), expectedMetadata) {
					t.Fatalf(`Error: expected '%s' to contain '%s'`, txt, expectedMetadata)
				}

				expectedText := "echo text from cache launch layer"
				if txt, err := ioutil.ReadFile(filepath.Join(layersDir, "buildpack.id", "cache-launch", "file-from-cache-launch-layer")); err != nil {
					t.Fatalf("failed to read file-from-cache-launch-layer: %s", err)
				} else if !strings.Contains(string(txt), expectedText) {
					t.Fatalf(`Error: expected '%s' to contain '%s'`, txt, expectedText)
				}

				if sha, err := ioutil.ReadFile(filepath.Join(layersDir, "buildpack.id", "cache-launch.sha")); err != nil {
					t.Fatalf("failed to read cache-launch.sha: %s", err)
				} else if string(sha) != cacheLaunchLayerSHA {
					t.Fatalf(`Error: expected '%s' to be equal to '%s'`, sha, cacheLaunchLayerSHA)
				}
			})

			it("doesn't restore cache false layers", func() {
				h.AssertNil(t, restorer.Restore(testCache))
				if _, err := os.Stat(filepath.Join(layersDir, "buildpack.id", "cache-false.toml")); !os.IsNotExist(err) {
					t.Fatal("Error: cache-false.toml should not have been restored")
				}

				if _, err := os.Stat(filepath.Join(layersDir, "buildpack.id", "cache-false")); !os.IsNotExist(err) {
					t.Fatal("Error: cache-false layer dir should not have been restored")
				}
			})

			it("doesn't restore layers from buildpacks that aren't in the group", func() {
				h.AssertNil(t, restorer.Restore(testCache))
				if _, err := os.Stat(filepath.Join(layersDir, "nogroup.buildpack.id")); !os.IsNotExist(err) {
					t.Fatal("Error: expected nogroup.buildpack.id not to be restored")
				}
			})

			it("escapes buildpack IDs when restoring buildpack layers", func() {
				h.AssertNil(t, restorer.Restore(testCache))
				expectedMetadata := `[metadata]
  escaped-bp-key = "escaped-bp-val"`
				if txt, err := ioutil.ReadFile(filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer.toml")); err != nil {
					t.Fatalf("failed to read escaped-bp-layer.toml: %s", err)
				} else if !strings.Contains(string(txt), expectedMetadata) {
					t.Fatalf(`Error: expected '%s' to contain '%s'`, txt, expectedMetadata)
				}

				expectedText := "echo text from escaped bp layer"
				if txt, err := ioutil.ReadFile(filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer", "file-from-escaped-bp")); err != nil {
					t.Fatalf("failed to read file-from-escaped-bp: %s", err)
				} else if !strings.Contains(string(txt), expectedText) {
					t.Fatalf(`Error: expected '%s' to contain '%s'`, txt, expectedText)
				}
			})

			when("restorer is running as root", func() {
				it.Before(func() {
					if os.Getuid() != 0 {
						t.Skip()
					}
				})

				it("recursively chowns the layers dir to CNB_USER_ID:CNB_GROUP_ID", func() {
					h.AssertNil(t, restorer.Restore(testCache))
					h.AssertUIDGID(t, layersDir, 1234, 4321)
					h.AssertUIDGID(t, filepath.Join(layersDir, "buildpack.id"), 1234, 4321)
					h.AssertUIDGID(t, filepath.Join(layersDir, "buildpack.id", "cache-launch"), 1234, 4321)
					h.AssertUIDGID(t, filepath.Join(layersDir, "buildpack.id", "cache-launch", "file-from-cache-launch-layer"), 1234, 4321)
				})
			})
		})
	})
}

func addLayerFromPath(t *testing.T, tarTempDir, layerPath string, c lifecycle.Cache) string {
	t.Helper()
	tarPath := filepath.Join(tarTempDir, h.RandString(10)+".tar")
	sha, err := archive.WriteTarFile(layerPath, tarPath, 0, 0)
	h.AssertNil(t, err)
	h.AssertNil(t, c.AddLayerFile(sha, tarPath))
	return sha
}
