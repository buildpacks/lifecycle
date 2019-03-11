package lifecycle_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/archive"
	"github.com/buildpack/lifecycle/image"
	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestRestorer(t *testing.T) {
	spec.Run(t, "Restorer", testRestorer, spec.Report(report.Terminal{}))
}

func testRestorer(t *testing.T, when spec.G, it spec.S) {
	when("#Restore", func() {
		var (
			restorer  *lifecycle.Restorer
			layersDir string
		)

		it.Before(func() {
			var err error
			layersDir, err = ioutil.TempDir("", "lifecycle-layer-dir")
			h.AssertNil(t, err)

			restorer = &lifecycle.Restorer{
				LayersDir: layersDir,
				Buildpacks: []*lifecycle.Buildpack{
					{ID: "buildpack.id"},
					{ID: "escaped/buildpack/id"},
				},
				Out: log.New(ioutil.Discard, "", 0),
			}
		})

		it.After(func() {
			h.AssertNil(t, os.RemoveAll(layersDir))
		})

		when("there is no cache image", func() {
			var cacheImage image.Image

			it.Before(func() {
				factory, err := image.NewFactory()
				h.AssertNil(t, err)
				cacheImage, err = factory.NewLocal("not-exist")
				h.AssertNil(t, err)
			})

			it("does nothing", func() {
				h.AssertNil(t, restorer.Restore(cacheImage))
			})
		})

		when("there is a cache image", func() {
			var (
				cacheImage          *h.FakeImage
				tarTempDir          string
				cacheOnlyLayerSHA   string
				cacheLaunchLayerSHA string
				noGroupLayerSHA     string
				cacheFalseLayerSHA  string
				escapedLayerSHA     string
			)

			it.Before(func() {
				h.AssertNil(t, os.Setenv("PACK_USER_ID", "1234"))
				h.AssertNil(t, os.Setenv("PACK_GROUP_ID", "4321"))
				h.RecursiveCopy(t, filepath.Join("testdata", "restorer"), layersDir)
				var err error

				cacheImage = h.NewFakeImage(t, "cache-image-name", "", "")

				tarTempDir, err = ioutil.TempDir("", "restorer-test-temp-layer")
				h.AssertNil(t, err)

				cacheOnlyLayerSHA = addLayerFromPath(
					t,
					tarTempDir,
					filepath.Join(layersDir, "buildpack.id", "cache-only"),
					cacheImage,
				)

				cacheFalseLayerSHA = addLayerFromPath(
					t,
					tarTempDir,
					filepath.Join(layersDir, "buildpack.id", "cache-false"),
					cacheImage,
				)

				cacheLaunchLayerSHA = addLayerFromPath(
					t,
					tarTempDir,
					filepath.Join(layersDir, "buildpack.id", "cache-launch"),
					cacheImage,
				)

				noGroupLayerSHA = addLayerFromPath(
					t,
					tarTempDir,
					filepath.Join(layersDir, "nogroup.buildpack.id", "some-layer"),
					cacheImage,
				)

				escapedLayerSHA = addLayerFromPath(
					t,
					tarTempDir,
					filepath.Join(layersDir, "escaped_buildpack_id", "escaped-bp-layer"),
					cacheImage,
				)

				h.AssertNil(t, os.RemoveAll(layersDir))
				h.AssertNil(t, os.Mkdir(layersDir, 0777))

				cacheImage.SetLabel("io.buildpacks.lifecycle.cache.metadata", fmt.Sprintf(`{
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
				}`, cacheOnlyLayerSHA, cacheFalseLayerSHA, cacheLaunchLayerSHA, noGroupLayerSHA, escapedLayerSHA))
			})

			it.After(func() {
				os.RemoveAll(tarTempDir)
			})

			it("restores cached layers", func() {
				h.AssertNil(t, restorer.Restore(cacheImage))
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
				h.AssertNil(t, restorer.Restore(cacheImage))
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
				h.AssertNil(t, restorer.Restore(cacheImage))
				if _, err := os.Stat(filepath.Join(layersDir, "buildpack.id", "cache-false.toml")); !os.IsNotExist(err) {
					t.Fatal("Error: cache-false.toml should not have been restored")
				}

				if _, err := os.Stat(filepath.Join(layersDir, "buildpack.id", "cache-false")); !os.IsNotExist(err) {
					t.Fatal("Error: cache-false layer dir should not have been restored")
				}
			})

			it("doesn't restore layers from buildpacks that aren't in the group", func() {
				h.AssertNil(t, restorer.Restore(cacheImage))
				if _, err := os.Stat(filepath.Join(layersDir, "nogroup.buildpack.id")); !os.IsNotExist(err) {
					t.Fatal("Error: expected nogroup.buildpack.id not to be restored")
				}
			})

			it("escapes buildpack IDs when restoring buildpack layers", func() {
				h.AssertNil(t, restorer.Restore(cacheImage))
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

				it("recursively chowns the layers dir to PACK_USER_ID:PACK_GROUP_ID", func() {
					h.AssertNil(t, restorer.Restore(cacheImage))
					assertUidGid(t, layersDir, 1234, 4321)
					assertUidGid(t, filepath.Join(layersDir, "buildpack.id"), 1234, 4321)
					assertUidGid(t, filepath.Join(layersDir, "buildpack.id", "cache-launch"), 1234, 4321)
					assertUidGid(t, filepath.Join(layersDir, "buildpack.id", "cache-launch", "file-from-cache-launch-layer"), 1234, 4321)
				})
			})
		})
	})
}

func assertUidGid(t *testing.T, path string, uid, gid int) {
	fi, err := os.Stat(path)
	h.AssertNil(t, err)
	stat := fi.Sys().(*syscall.Stat_t)
	h.AssertEq(t, stat.Uid, uint32(uid))
	h.AssertEq(t, stat.Gid, uint32(gid))
}

func addLayerFromPath(t *testing.T, tarTempDir string, layerPath string, cacheImage *h.FakeImage) string {
	t.Helper()
	tarPath := filepath.Join(tarTempDir, h.RandString(10)+".tar")
	sha, err := archive.WriteTarFile(layerPath, tarPath, 0, 0)
	h.AssertNil(t, err)
	h.AssertNil(t, cacheImage.AddLayer(tarPath))
	return sha
}
