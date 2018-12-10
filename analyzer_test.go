package lifecycle_test

import (
	"bytes"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	h "github.com/buildpack/lifecycle/testhelpers"
	"github.com/buildpack/lifecycle/testmock"
)

func TestAnalyzer(t *testing.T) {
	spec.Run(t, "Analyzer", testAnalyzer, spec.Report(report.Terminal{}))
}

//go:generate mockgen -mock_names Image=GGCRImage -package testmock -destination testmock/image.go github.com/google/go-containerregistry/pkg/v1 Image
//go:generate mockgen -package testmock -destination testmock/ref.go github.com/google/go-containerregistry/pkg/name Reference
//go:generate mockgen -package testmock -destination testmock/image.go github.com/buildpack/lifecycle/image Image

func testAnalyzer(t *testing.T, when spec.G, it spec.S) {
	var (
		analyzer       *lifecycle.Analyzer
		mockCtrl       *gomock.Controller
		stdout, stderr *bytes.Buffer
		layerDir       string
	)

	it.Before(func() {
		var err error
		layerDir, err = ioutil.TempDir("", "lifecycle-layer-dir")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}

		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
		analyzer = &lifecycle.Analyzer{
			Buildpacks: []*lifecycle.Buildpack{{ID: "buildpack.node"}, {ID: "buildpack.go"}, {ID: "no.metadata.buildpack"}},
			Out:        log.New(stdout, "", 0),
			Err:        log.New(stderr, "", 0),
		}
		mockCtrl = gomock.NewController(t)
	})

	it.After(func() {
		os.RemoveAll(layerDir)
		mockCtrl.Finish()
	})

	when("Analyze", func() {
		var (
			image *testmock.MockImage
			ref   *testmock.MockReference
		)
		it.Before(func() {
			image = testmock.NewMockImage(mockCtrl)
			ref = testmock.NewMockReference(mockCtrl)
			ref.EXPECT().Name().AnyTimes()
		})

		when("image exists", func() {
			when("image label has compatible metadata", func() {
				it.Before(func() {
					image.EXPECT().Found().Return(true, nil)
					image.EXPECT().Label("io.buildpacks.lifecycle.metadata").Return(`{
  "buildpacks": [
    {
      "key": "buildpack.node",
      "layers": {
        "nodejs": {
          "data": {
            "akey": "avalue",
            "bkey": "bvalue"
          },
          "sha": "nodejs-layer-sha"
        },
        "node_modules": {
          "data": {
            "version": "1234"
          },
          "sha": "node-modules-sha"
        },
        "buildhelpers": {
          "data": {
            "some": "metadata"
          },
          "sha": "new-buildhelpers-sha",
          "build": true
        }
      }
    },
    {
      "key": "buildpack.go",
      "layers": {
        "go": {
          "data": {
            "version": "1.10"
          }
        }
      }
    }
  ]
}`, nil)
				})

				it("should use labels to populate the layer dir", func() {
					if err := analyzer.Analyze(image, layerDir); err != nil {
						t.Fatalf("Error: %s\n", err)
					}

					for _, data := range []struct{ name, expected string }{
						{"buildpack.node/nodejs.toml", `[metadata]
  akey = "avalue"
  bkey = "bvalue"`},
						{"buildpack.node/node_modules.toml", `[metadata]
  version = "1234"`},
						{"buildpack.go/go.toml", `[metadata]
  version = "1.10"`},
					} {
						if txt, err := ioutil.ReadFile(filepath.Join(layerDir, data.name)); err != nil {
							t.Fatalf("Error: %s\n", err)
						} else if !strings.Contains(string(txt), data.expected) {
							t.Fatalf(`Error: expected "%s" to contain "%s"`, txt, data.expected)
						}
					}
				})

				it("should only write layer TOML files that correspond to detected buildpacks", func() {
					analyzer.Buildpacks = []*lifecycle.Buildpack{{ID: "buildpack.go"}}

					if err := analyzer.Analyze(image, layerDir); err != nil {
						t.Fatalf("Error: %s\n", err)
					}

					if txt, err := ioutil.ReadFile(filepath.Join(layerDir, "buildpack.go", "go.toml")); err != nil {
						t.Fatalf("Error: %s\n", err)
					} else if !strings.Contains(string(txt), `[metadata]
  version = "1.10"`) {
						t.Fatalf(`Error: expected "%s" to be toml encoded go.toml`, txt)
					}

					if _, err := os.Stat(filepath.Join(layerDir, "buildpack.node")); !os.IsNotExist(err) {
						t.Fatalf(`Error: expected /layer/buildpack.node to not exist`)
					}
				})

				when("there are cached launch layers", func() {
					it("leaves the layers", func() {
						//copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if err := analyzer.Analyze(image, layerDir); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						if txt, err := ioutil.ReadFile(filepath.Join(layerDir, "buildpack.node", "nodejs", "node-file")); err != nil {
							t.Fatalf("Error: %s\n", err)
						} else if string(txt) != "nodejs cached file" {
							t.Fatalf("Error: expected cached node file to remain")
						}
					})
				})

				when("there are cached build layers", func() {
					it("leaves the layers", func() {
						//copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if err := analyzer.Analyze(image, layerDir); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						if txt, err := ioutil.ReadFile(filepath.Join(layerDir, "buildpack.node", "build-layer", "build-layer-file")); err != nil {
							t.Fatalf("Error: %s\n", err)
						} else if string(txt) != "build-layer-file-contents" {
							t.Fatalf("Error: expected cached node file to remain")
						}
					})
				})

				when("there are stale cached launch layers", func() {
					it("removes the layer dir and rewrites the metadata", func() {
						//copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if err := analyzer.Analyze(image, layerDir); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						var err error
						if _, err = ioutil.ReadDir(filepath.Join(layerDir, "buildpack.node", "node_modules")); !os.IsNotExist(err) {
							t.Fatalf("Found stale node_modules layer dir, it should not exist")
						}
						if txt, err := ioutil.ReadFile(filepath.Join(layerDir, "buildpack.node", "node_modules.toml")); err != nil {
							t.Fatalf("failed to read node_modules.toml: %s", err)
						} else if !strings.Contains(string(txt), `[metadata]
  version = "1234"`) {
							t.Fatalf(`Error: expected "%s" to be equal %s`, txt, `metadata.version = "1234"`)
						}

						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "buildpack.node", "node_modules.sha")); !os.IsNotExist(err) {
							t.Fatalf("Found stale node_modules.sha, it should be removed")
						}
					})
				})

				when("there are stale cached launch/build layers", func() {
					it("removes the layer dir and metadata", func() {
						//copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if err := analyzer.Analyze(image, layerDir); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						var err error
						if _, err = ioutil.ReadDir(filepath.Join(layerDir, "buildpack.node", "buildhelpers")); !os.IsNotExist(err) {
							t.Fatalf("Found stale buildhelpers layer dir, it should not exist")
						}

						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "buildpack.node", "buildhelpers.toml")); !os.IsNotExist(err) {
							t.Fatalf("Found stale buildhelpers.toml, it should be removed")
						}

						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "buildpack.node", "buildhelpers.sha")); !os.IsNotExist(err) {
							t.Fatalf("Found stale buildhelpers.sha, it should be removed")
						}
					})
				})

				when("there cached launch layers that are missing from metadata", func() {
					it("removes the layer dir and metadata", func() {
						//copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if err := analyzer.Analyze(image, layerDir); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						var err error
						if _, err = ioutil.ReadDir(filepath.Join(layerDir, "buildpack.node", "old-layer")); !os.IsNotExist(err) {
							t.Fatalf("Found stale old-layer layer dir, it should not exist")
						}

						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "buildpack.node", "old-layer.toml")); !os.IsNotExist(err) {
							t.Fatalf("Found stale old-layer.toml, it should be removed")
						}

						if _, err := ioutil.ReadFile(filepath.Join(layerDir, "buildpack.node", "old-layer.sha")); !os.IsNotExist(err) {
							t.Fatalf("Found stale old-layer.sha, it should be removed")
						}
					})
				})

				when("there are cached layers for a buildpack that is missing from the group", func() {
					it("removes all the layers", func() {
						//copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if err := analyzer.Analyze(image, layerDir); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						var err error
						if _, err = ioutil.ReadDir(filepath.Join(layerDir, "old-buildpack")); !os.IsNotExist(err) {
							t.Fatalf("Found old-buildpack dir, it should not exist")
						}
					})

					it("does not remove app layers", func() {
						//copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if err := analyzer.Analyze(image, layerDir); err != nil {
							t.Fatalf("Error: %s\n", err)
						}

						appFile := filepath.Join(layerDir, "app", "appfile")
						if txt, err := ioutil.ReadFile(appFile); err != nil {
							t.Fatalf("Error: %s\n", err)
						} else if !strings.Contains(string(txt), "appFile file contents") {
							t.Fatalf(`Error: expected "%s" to still exist`, appFile)
						}
					})

					it("does not remove remaining layerDir files", func() {
						//copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if err := analyzer.Analyze(image, layerDir); err != nil {
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
						//copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if err := analyzer.Analyze(image, layerDir); err != nil {
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
						//copy to layerDir
						h.RecursiveCopy(t, filepath.Join("testdata", "analyzer", "cached-layers"), layerDir)

						if err := analyzer.Analyze(image, layerDir); err != nil {
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
			})
		})

		when("the image cannot found", func() {
			it.Before(func() {
				image.EXPECT().Found().Return(false, nil)
				image.EXPECT().Name().Return("test-name")
			})

			it("warns user and returns", func() {
				err := analyzer.Analyze(image, layerDir)
				assertNil(t, err)
				if !strings.Contains(stdout.String(), "WARNING: skipping analyze, image 'test-name' not found or requires authentication to access") {
					t.Fatalf("expected warning in stdout: %s", stdout.String())
				}
			})
		})

		when("there is an error while trying to find the image", func() {
			it.Before(func() {
				image.EXPECT().Found().Return(false, errors.New("some-error"))
			})

			it("returns the error", func() {
				err := analyzer.Analyze(image, layerDir)
				h.AssertError(t, err, "some-error")
			})
		})

		when("the image does not have the required label", func() {
			it.Before(func() {
				image.EXPECT().Found().Return(true, nil)
				image.EXPECT().Label("io.buildpacks.lifecycle.metadata").Return("", nil)
			})

			it("warns user and returns", func() {
				err := analyzer.Analyze(image, layerDir)
				assertNil(t, err)

				if !strings.Contains(stdout.String(), "WARNING: skipping analyze, previous image metadata was not found") {
					t.Fatalf("expected warning in stdout: %s", stdout.String())
				}
			})
		})

		when("the image label has incompatible metadata", func() {
			it.Before(func() {
				image.EXPECT().Found().Return(true, nil)
				image.EXPECT().Label("io.buildpacks.lifecycle.metadata").Return(`{["bad", "metadata"]}`, nil)

			})

			it("warns user and returns", func() {
				err := analyzer.Analyze(image, layerDir)
				assertNil(t, err)

				if !strings.Contains(stdout.String(), "WARNING: skipping analyze, previous image metadata was incompatible") {
					t.Fatalf("expected warning in stdout: %s", stdout.String())
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

func assertEq(t *testing.T, actual, expected interface{}) {
	t.Helper()
	if diff := cmp.Diff(actual, expected); diff != "" {
		t.Fatal(diff)
	}
}
