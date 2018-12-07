package lifecycle_test

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/testhelpers"
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
		launchDir      string
	)

	it.Before(func() {
		var err error
		launchDir, err = ioutil.TempDir("", "lifecycle-launch-dir")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}

		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
		analyzer = &lifecycle.Analyzer{
			Buildpacks: []*lifecycle.Buildpack{{ID: "buildpack.node"}, {ID: "buildpack.go"}},
			Out:        io.MultiWriter(stdout, it.Out()),
			Err:        io.MultiWriter(stderr, it.Out()),
		}
		mockCtrl = gomock.NewController(t)
	})

	it.After(func() {
		os.RemoveAll(launchDir)
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
					image.EXPECT().Label("io.buildpacks.lifecycle.metadata").Return(`
{
  "buildpacks": [
    {
      "key": "buildpack.node",
      "layers": {
        "nodejs": {
          "data": {
            "akey": "avalue",
            "bkey": "bvalue"
          }
        },
        "node_modules": {
          "data": {
            "version": "1234"
          }
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

				it("should use labels to populate the launch dir", func() {
					if err := analyzer.Analyze(image, launchDir); err != nil {
						t.Fatalf("Error: %s\n", err)
					}

					for _, data := range []struct{ name, expected string }{
						{"buildpack.node/nodejs.toml", `akey = "avalue"` + "\n" + `bkey = "bvalue"` + "\n"},
						{"buildpack.node/node_modules.toml", `version = "1234"` + "\n"},
						{"buildpack.go/go.toml", `version = "1.10"` + "\n"},
					} {
						if txt, err := ioutil.ReadFile(filepath.Join(launchDir, data.name)); err != nil {
							t.Fatalf("Error: %s\n", err)
						} else if string(txt) != data.expected {
							t.Fatalf(`Error: expected "%s" to be toml encoded %s`, txt, data.name)
						}
					}
				})

				it("should only write layer TOML files that correspond to detected buildpacks", func() {
					analyzer.Buildpacks = []*lifecycle.Buildpack{{ID: "buildpack.go"}}

					if err := analyzer.Analyze(image, launchDir); err != nil {
						t.Fatalf("Error: %s\n", err)
					}

					if txt, err := ioutil.ReadFile(filepath.Join(launchDir, "buildpack.go", "go.toml")); err != nil {
						t.Fatalf("Error: %s\n", err)
					} else if string(txt) != `version = "1.10"`+"\n" {
						t.Fatalf(`Error: expected "%s" to be toml encoded go.toml`, txt)
					}

					if _, err := os.Stat(filepath.Join(launchDir, "buildpack.node")); !os.IsNotExist(err) {
						t.Fatalf(`Error: expected /launch/buildpack.node to not exist`)
					}
				})
			})
		})

		when("the image cannot found", func() {
			it.Before(func() {
				image.EXPECT().Found().Return(false, nil)
				image.EXPECT().Name().Return("test-name")
			})

			it("warns user and returns", func() {
				err := analyzer.Analyze(image, launchDir)
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
				err := analyzer.Analyze(image, launchDir)
				testhelpers.AssertError(t, err, "some-error")
			})
		})

		when("the image does not have the required label", func() {
			it.Before(func() {
				image.EXPECT().Found().Return(true, nil)
				image.EXPECT().Label("io.buildpacks.lifecycle.metadata").Return("", nil)
			})

			it("warns user and returns", func() {
				err := analyzer.Analyze(image, launchDir)
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
				err := analyzer.Analyze(image, launchDir)
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
