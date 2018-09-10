package lifecycle_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/testmock"
	"github.com/golang/mock/gomock"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestAnalyzer(t *testing.T) {
	spec.Run(t, "Analyzer", testAnalyzer, spec.Report(report.Terminal{}))
}

//go:generate mockgen -package testmock -destination testmock/image.go github.com/google/go-containerregistry/pkg/v1 Image

func testAnalyzer(t *testing.T, when spec.G, it spec.S) {
	var (
		analyzer       *lifecycle.Analyzer
		mockCtrl       *gomock.Controller
		stdout, stderr *bytes.Buffer
		launchDir      string
		image          *testmock.MockImage
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		image = testmock.NewMockImage(mockCtrl)

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
	})

	it.After(func() {
		os.RemoveAll(launchDir)
		mockCtrl.Finish()
	})

	when("#Analyze", func() {
		when("image exists and has labels", func() {
			it.Before(func() {
				var configFile = &v1.ConfigFile{}
				configFile.Config.Labels = map[string]string{lifecycle.MetadataLabel: `{
					"buildpacks": [
						{
							"key": "buildpack.node",
							"layers": {
								"nodejs": {
									"data": {"akey": "avalue", "bkey": "bvalue"}
								},
								"node_modules": {
									"data": {"version": "1234"}
								}
							}
						},
						{
							"key": "buildpack.go",
							"layers": {
								"go": {
									"data": {"version": "1.10"}
								}
							}
						}
					]
				}`}
				image.EXPECT().ConfigFile().Return(configFile, nil)
			})

			it("should use labels to populate the launch dir", func() {
				if err := analyzer.Analyze(launchDir, image); err != nil {
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
		})

		when("image exists but is missing config", func() {
			it.Before(func() {
				var configFile = &v1.ConfigFile{}
				image.EXPECT().ConfigFile().Return(configFile, nil)
			})

			it("should do nothing and succeed", func() {
				if err := analyzer.Analyze(launchDir, image); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
			})
		})

		when("image has buildpacks that won't be run", func() {
			it.Before(func() {
				var configFile = &v1.ConfigFile{}
				configFile.Config.Labels = map[string]string{lifecycle.MetadataLabel: `{
					"buildpacks": [
						{
							"key": "buildpack.node",
							"layers": {
								"node_modules": {
									"data": {"version": "1234"}
								}
							}
						},
						{
							"key": "buildpack.go",
							"layers": {
								"go": {
									"data": {"version": "1.10"}
								}
							}
						}
					]
				}`}
				image.EXPECT().ConfigFile().Return(configFile, nil)
			})

			it("should only write layer TOML files that correspond to detected buildpacks", func() {
				analyzer.Buildpacks = []*lifecycle.Buildpack{{ID: "buildpack.go"}}

				if err := analyzer.Analyze(launchDir, image); err != nil {
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
}
