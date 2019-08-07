package lifecycle_test

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildpack/imgutil/fakes"
	"github.com/buildpack/imgutil/local"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/metadata"
	h "github.com/buildpack/lifecycle/testhelpers"
	"github.com/buildpack/lifecycle/testmock"
)

func TestAnalyzer(t *testing.T) {
	spec.Run(t, "Analyzer", testAnalyzer, spec.Report(report.Terminal{}))
}

//go:generate mockgen -mock_names Image=GGCRImage -package testmock -destination testmock/image.go github.com/google/go-containerregistry/pkg/v1 Image
//go:generate mockgen -package testmock -destination testmock/ref.go github.com/google/go-containerregistry/pkg/name Reference

func testAnalyzer(t *testing.T, when spec.G, it spec.S) {
	var (
		analyzer       *lifecycle.Analyzer
		mockCtrl       *gomock.Controller
		stdout, stderr *bytes.Buffer
		layerDir       string
		appDir         string
		tmpDir         string
	)

	it.Before(func() {
		var err error

		tmpDir, err = ioutil.TempDir("", "analyzer-tests")
		h.AssertNil(t, err)

		layerDir, err = ioutil.TempDir("", "lifecycle-layer-dir")
		h.AssertNil(t, err)

		appDir = filepath.Join(layerDir, "some-app-dir")
		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
		analyzer = &lifecycle.Analyzer{
			Buildpacks:   []lifecycle.Buildpack{{ID: "metdata.buildpack"}, {ID: "no.cache.buildpack"}, {ID: "no.metadata.buildpack"}},
			AppDir:       appDir,
			LayersDir:    layerDir,
			AnalyzedPath: filepath.Join(tmpDir, "some-previous-file.toml"),
			Out:          log.New(stdout, "", 0),
			Err:          log.New(stderr, "", 0),
			UID:          1234,
			GID:          4321,
		}
		mockCtrl = gomock.NewController(t)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
		os.RemoveAll(layerDir)
		mockCtrl.Finish()
	})

	when("#Analyze", func() {
		var (
			image            *fakes.Image
			appImageMetadata metadata.LayersMetadata
			ref              *testmock.MockReference
		)

		it.Before(func() {
			image = fakes.NewImage("image-repo-name", "", local.IDIdentifier{
				ImageID: "s0m3D1g3sT",
			})
			ref = testmock.NewMockReference(mockCtrl)
			ref.EXPECT().Name().AnyTimes()
		})

		it.After(func() {
			image.Cleanup()
		})

		when("image exists", func() {
			when("image label has compatible metadata", func() {
				it.Before(func() {
					metadataLabel := `{
  "buildpacks": [
    {
      "key": "metdata.buildpack",
      "layers": {
        "valid-launch": {
          "data": {
            "akey": "avalue",
            "bkey": "bvalue"
          },
          "sha": "valid-launch-layer-sha",
          "launch": true
        },
        "valid-launch-build": {
          "data": {
            "some-key": "val-from-metadata",
            "some-other-key": "val-from-metadata"
          },
          "sha": "valid-launch-build-sha",
          "launch": true,
          "build": true
        },
        "stale-launch": {
          "data": {
            "version": "1234"
          },
          "sha": "new-sha",
          "launch": true
        },
        "stale-launch-build": {
          "data": {
            "some": "metadata"
          },
          "sha": "new-launch-build-sha",
          "build": true,
          "launch": true
        },
        "launch-cache": {
          "data": {
            "some": "metadata"
          },
          "sha": "launch-cache-sha",
          "cache": true,
          "launch": true
        }
      }
    },
    {
      "key": "no.cache.buildpack",
      "layers": {
        "go": {
          "data": {
            "version": "1.10"
          }
        }
      }
    }
  ]
}`
					h.AssertNil(t,
						image.SetLabel(
							"io.buildpacks.lifecycle.metadata",
							metadataLabel,
						))
					h.AssertNil(t, json.Unmarshal([]byte(metadataLabel), &appImageMetadata))
				})

				it("should use labels to populate the layer dir", func() {
					if _, err := analyzer.Analyze(image); err != nil {
						t.Fatalf("Error: %s\n", err)
					}

					for _, data := range []struct{ name, expected string }{
						{"metdata.buildpack/valid-launch.toml", `[metadata]
  akey = "avalue"
  bkey = "bvalue"`},
						{"metdata.buildpack/stale-launch.toml", `[metadata]
  version = "1234"`},
						{"no.cache.buildpack/go.toml", `[metadata]
  version = "1.10"`},
					} {
						if txt, err := ioutil.ReadFile(filepath.Join(layerDir, data.name)); err != nil {
							t.Fatalf("Error: %s\n", err)
						} else if !strings.Contains(string(txt), data.expected) {
							t.Fatalf(`Error: expected "%s" to contain "%s"`, string(txt), data.expected)
						}
					}
				})

				it("should only write layer TOML files that correspond to detected buildpacks", func() {
					analyzer.Buildpacks = []lifecycle.Buildpack{{ID: "no.cache.buildpack"}}

					if _, err := analyzer.Analyze(image); err != nil {
						t.Fatalf("Error: %s\n", err)
					}

					if txt, err := ioutil.ReadFile(filepath.Join(layerDir, "no.cache.buildpack", "go.toml")); err != nil {
						t.Fatalf("Error: %s\n", err)
					} else if !strings.Contains(string(txt), `[metadata]
  version = "1.10"`) {
						t.Fatalf(`Error: expected "%s" to be toml encoded go.toml`, txt)
					}

					if _, err := os.Stat(filepath.Join(layerDir, "metdata.buildpack")); !os.IsNotExist(err) {
						t.Fatalf(`Error: expected /layer/metdata.buildpack to not exist`)
					}
				})

				it("should return the analyzed metadata", func() {
					md, err := analyzer.Analyze(image)
					h.AssertNil(t, err)

					h.AssertEq(t, md.Image.Reference, "s0m3D1g3sT")
					h.AssertEq(t, md.Metadata, appImageMetadata)
				})

				when("there is a launch/build layer that isn't cached", func() {
					it("should not restore the metadata", func() {
						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}
						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack/stale-launch-build.toml")); !os.IsNotExist(err) {
							t.Fatalf("Found unexpected metadata for stale-launch-build layer")
						}
					})
				})

				when("there is a cache=true layer in the metadata but not in the cache", func() {
					it("should not restore the metadata", func() {
						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}
						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack", "launch-cache.toml")); !os.IsNotExist(err) {
							t.Fatalf("Found unexpected metadata for launch-cache layer")
						}
					})
				})

				when("there are cached launch layers", func() {
					it("leaves the layers", func() {
						// copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						if txt, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack", "valid-launch", "valid-launch-file")); err != nil {
							t.Fatalf("Error: %s\n", err)
						} else if string(txt) != "valid-launch cached file" {
							t.Fatalf("Error: expected cached node file to remain")
						}
					})
				})

				when("there are cached launch layers", func() {
					it("leaves the layer dir and updates the metadata", func() {
						// copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						if txt, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack", "valid-launch.toml")); err != nil {
							t.Fatalf("Error: %s\n", err)
						} else {
							expected := `
[metadata]
  akey = "avalue"
  bkey = "bvalue"
`
							if !strings.Contains(string(txt), expected) {
								t.Fatalf(`Error: expected "%s" to contain "%s"`, string(txt), expected)
							}
						}
					})
				})

				when("there are cached launch/build layers", func() {
					it("leaves the layer dir and updates the metadata", func() {
						// copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						if txt, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack", "valid-launch-build.toml")); err != nil {
							t.Fatalf("Error: %s\n", err)
						} else {
							expected := `
[metadata]
  some-key = "val-from-metadata"
  some-other-key = "val-from-metadata"`
							if !strings.Contains(string(txt), expected) {
								t.Fatalf("Error: expected metadata to be rewritten \nExpected:\n%s\n\nTo Contain:\n"+
									"%s\n", string(txt), expected)
							}
						}
					})
				})

				when("there are cached build layers", func() {
					it("leaves the layers", func() {
						// copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						if txt, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack", "build-layer", "build-layer-file")); err != nil {
							t.Fatalf("Error: %s\n", err)
						} else if string(txt) != "build-layer-file-contents" {
							t.Fatalf("Error: expected cached node file to remain")
						}
					})
				})

				when("there are stale cached launch layers", func() {
					it("removes the layer dir and rewrites the metadata", func() {
						// copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						var err error
						if _, err = ioutil.ReadDir(filepath.Join(layerDir, "metdata.buildpack", "node_modules")); !os.IsNotExist(err) {
							t.Fatalf("Found stale node_modules layer dir, it should not exist")
						}
						if txt, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack", "stale-launch.toml")); err != nil {
							t.Fatalf("failed to read stale-launch.toml: %s", err)
						} else if !strings.Contains(string(txt), `[metadata]
  version = "1234"`) {
							t.Fatalf(`Error: expected "%s" to be equal %s`, txt, `metadata.version = "1234"`)
						}

						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack", "stale-launch.sha")); !os.IsNotExist(err) {
							t.Fatalf("Found stale stale-launch.sha, it should be removed")
						}
					})
				})

				when("there are malformed layers", func() {
					it("removes the layer", func() {
						// copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						var err error
						if _, err = ioutil.ReadDir(filepath.Join(layerDir, "metdata.buildpack", "bad-layer")); !os.IsNotExist(err) {
							t.Fatalf("Found bad-layer layer dir, it should be removed")
						}
						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack", "bad-layer.toml")); !os.IsNotExist(err) {
							t.Fatalf("found bad-layer.toml, it should be removed")
						}
						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack", "bad-layer.sha")); !os.IsNotExist(err) {
							t.Fatalf("Found stale bad-layer.sha, it should be removed")
						}
					})
				})

				when("there are stale cached launch/build layers", func() {
					it("removes the layer dir and metadata", func() {
						// copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						var err error
						if _, err = ioutil.ReadDir(filepath.Join(layerDir, "metdata.buildpack", "stale-launch-build")); !os.IsNotExist(err) {
							t.Fatalf("Found stale stale-launch-build layer dir, it should not exist")
						}

						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack", "stale-launch-build.toml")); !os.IsNotExist(err) {
							t.Fatalf("Found stale stale-launch-build.toml, it should be removed")
						}

						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack", "stale-launch-build.sha")); !os.IsNotExist(err) {
							t.Fatalf("Found stale stale-launch-build.sha, it should be removed")
						}
					})
				})

				when("there cached launch layers that are missing from metadata", func() {
					it("removes the layer dir and metadata", func() {
						// copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						var err error
						if _, err = ioutil.ReadDir(filepath.Join(layerDir, "metdata.buildpack", "old-layer")); !os.IsNotExist(err) {
							t.Fatalf("Found stale old-layer layer dir, it should not exist")
						}

						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack", "old-layer.toml")); !os.IsNotExist(err) {
							t.Fatalf("Found stale old-layer.toml, it should be removed")
						}

						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "metdata.buildpack", "old-layer.sha")); !os.IsNotExist(err) {
							t.Fatalf("Found stale old-layer.sha, it should be removed")
						}
					})
				})

				when("there are cached layers for a buildpack that is missing from the group", func() {
					it("does not remove app layers", func() {
						// copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						appFile := filepath.Join(layerDir, "some-app-dir", "appfile")
						if txt, err := ioutil.ReadFile(appFile); err != nil {
							t.Fatalf("Error: %s\n", err)
						} else if !strings.Contains(string(txt), "appFile file contents") {
							t.Fatalf(`Error: expected "%s" to still exist`, appFile)
						}
					})

					it("does not remove remaining layerDir files", func() {
						// copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						appFile := filepath.Join(layerDir, "config.toml")
						if txt, err := ioutil.ReadFile(appFile); err != nil {
							t.Fatalf("Error: %s\n", err)
						} else if !strings.Contains(string(txt), "someNoneLayer = \"file\"") {
							t.Fatalf(`Error: expected "%s" to still exist`, appFile)
						}
					})
				})

				when("there are cached non launch layers for a buildpack that is missing from metadata", func() {
					it("keeps the layers", func() {
						// copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						buildLayerFile := filepath.Join(layerDir, "no.metadata.buildpack", "buildlayer", "buildlayerfile")
						if txt, err := ioutil.ReadFile(buildLayerFile); err != nil {
							t.Fatalf("Error: %s\n", err)
						} else if !strings.Contains(string(txt), "buildlayer file contents") {
							t.Fatalf(`Error: expected "%s" to still exist`, buildLayerFile)
						}

					})
				})

				when("there are cached non launch for a buildpack that is missing from metadata", func() {
					it("removes the layers", func() {
						// copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if _, err := analyzer.Analyze(image); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						var err error
						if _, err = ioutil.ReadDir(filepath.Join(layerDir, "no.metadata.buildpack", "launchlayer")); !os.IsNotExist(err) {
							t.Fatalf("Found stale launchlayer layer dir, it should not exist")
						}

						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "no.metadata.buildpack", "launchlayer.toml")); !os.IsNotExist(err) {
							t.Fatalf("Found stale launchlayer.toml, it should be removed")
						}
					})
				})

				when("analyzer is running as root", func() {
					it.Before(func() {
						if os.Getuid() != 0 {
							t.Skip("Skipped when not running as root")
						}
					})

					it("chowns new files to CNB_USER_ID:CNB_GROUP_ID", func() {
						_, err := analyzer.Analyze(image)
						h.AssertNil(t, err)
						h.AssertUidGid(t, layerDir, 1234, 4321)
						h.AssertUidGid(t, filepath.Join(layerDir, "metdata.buildpack", "valid-launch.toml"), 1234, 4321)
						h.AssertUidGid(t, filepath.Join(layerDir, "no.cache.buildpack"), 1234, 4321)
						h.AssertUidGid(t, filepath.Join(layerDir, "no.cache.buildpack", "go.toml"), 1234, 4321)
					})
				})

				when("skip-layers is true", func() {
					it.Before(func() {
						analyzer.SkipLayers = true
					})

					it("should return the analyzed metadata", func() {
						md, err := analyzer.Analyze(image)
						h.AssertNil(t, err)

						h.AssertEq(t, md.Image.Reference, "s0m3D1g3sT")
						h.AssertEq(t, md.Metadata, appImageMetadata)
					})

					it("does not write buildpack layers", func() {
						_, err := analyzer.Analyze(image)
						h.AssertNil(t, err)

						fileInfos, err := ioutil.ReadDir(layerDir)
						h.AssertNil(t, err)

						h.AssertEq(t, len(fileInfos), 0)
					})
				})
			})
		})

		when("the image cannot be found", func() {
			var notFoundImage *fakes.Image

			it.Before(func() {
				notFoundImage = fakes.NewImage("image-repo-name", "", nil)
				h.AssertNil(t, notFoundImage.Delete())
			})

			it("clears the cached launch layers", func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)
				_, err := analyzer.Analyze(notFoundImage)
				assertNil(t, err)

				if _, err := ioutil.ReadDir(filepath.Join(layerDir, "no.metadata.buildpack", "launchlayer")); !os.IsNotExist(err) {
					t.Fatalf("Found stale launchlayer cache, it should not exist")
				}
				if _, err := ioutil.ReadDir(filepath.Join(layerDir, "metdata.buildpack", "stale-launch-build")); !os.IsNotExist(err) {
					t.Fatalf("Found stale stale-launch-build cache, it should not exist")
				}
				if _, err := ioutil.ReadDir(filepath.Join(layerDir, "some-app-dir")); err != nil {
					t.Fatalf("Missing some-app-dir")
				}
			})

			it("should return a nil image in the analyzed metadata", func() {
				md, err := analyzer.Analyze(notFoundImage)
				h.AssertNil(t, err)

				h.AssertNil(t, md.Image)
				h.AssertEq(t, md.Metadata, metadata.LayersMetadata{})
			})
		})

		when("the image does not have the required label", func() {
			it.Before(func() {
				h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", ""))
			})

			it("returns", func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

				_, err := analyzer.Analyze(image)
				assertNil(t, err)

				if _, err := ioutil.ReadDir(filepath.Join(layerDir, "no.metadata.buildpack", "launchlayer")); !os.IsNotExist(err) {
					t.Fatalf("Found stale launchlayer cache, it should not exist")
				}
				if _, err := ioutil.ReadDir(filepath.Join(layerDir, "metdata.buildpack", "stale-launch-build")); !os.IsNotExist(err) {
					t.Fatalf("Found stale stale-launch-build cache, it should not exist")
				}
				if _, err := ioutil.ReadDir(filepath.Join(layerDir, "some-app-dir")); err != nil {
					t.Fatalf("Missing some-app-dir")
				}
			})
		})

		when("the image label has incompatible metadata", func() {
			it.Before(func() {
				h.AssertNil(t, image.SetLabel("io.buildpacks.lifecycle.metadata", `{["bad", "metadata"]}`))
			})

			it("returns", func() {
				h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

				_, err := analyzer.Analyze(image)
				assertNil(t, err)

				if _, err := ioutil.ReadDir(filepath.Join(layerDir, "no.metadata.buildpack", "launchlayer")); !os.IsNotExist(err) {
					t.Fatalf("Found stale launchlayer cache, it should not exist")
				}
				if _, err := ioutil.ReadDir(filepath.Join(layerDir, "metdata.buildpack", "stale-launch-build")); !os.IsNotExist(err) {
					t.Fatalf("Found stale stale-launch-build cache, it should not exist")
				}
				if _, err := ioutil.ReadDir(filepath.Join(layerDir, "some-app-dir")); err != nil {
					t.Fatalf("Missing some-app-dir")
				}
			})
		})
	})
}

func assertNil(t *testing.T, actual interface{}) {
	t.Helper()
	if actual != nil {
		t.Fatalf("Expected nil: %s", actual)
	}
}
