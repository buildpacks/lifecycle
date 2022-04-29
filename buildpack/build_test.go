package buildpack_test

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

var latestBuildpackAPI = api.Buildpack.Latest()

func TestBuild(t *testing.T) {
	spec.Run(t, "Build", testBuild, spec.Report(report.Terminal{}))
}

//go:generate mockgen -package testmock -destination testmock/env.go github.com/buildpacks/lifecycle BuildEnv

func testBuild(t *testing.T, when spec.G, it spec.S) {
	var (
		bpTOML         buildpack.Descriptor
		mockCtrl       *gomock.Controller
		mockEnv        *testmock.MockBuildEnv
		stdout, stderr *bytes.Buffer
		tmpDir         string
		platformDir    string
		appDir         string
		layersDir      string
		buildpacksDir  string
		config         buildpack.BuildConfig
		logHandler     = memory.New()
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		mockEnv = testmock.NewMockBuildEnv(mockCtrl)

		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
		platformDir = filepath.Join(tmpDir, "platform")
		layersDir = filepath.Join(tmpDir, "launch")
		appDir = filepath.Join(layersDir, "app")
		h.Mkdir(t, layersDir, appDir, filepath.Join(platformDir, "env"))

		buildpacksDir, err = filepath.Abs(filepath.Join("testdata", "by-id"))
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}

		config = buildpack.BuildConfig{
			AppDir:      appDir,
			PlatformDir: platformDir,
			LayersDir:   layersDir,
			Out:         stdout,
			Err:         stderr,
			Logger:      &log.Logger{Handler: logHandler},
		}

		bpTOML = buildpack.Descriptor{
			API: latestBuildpackAPI.String(),
			Buildpack: buildpack.Info{
				ID:       "A",
				Version:  "v1",
				Name:     "Buildpack A",
				ClearEnv: false,
				Homepage: "Buildpack A Homepage",
			},
			Dir: filepath.Join(buildpacksDir, "A", "v1"),
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
		mockCtrl.Finish()
	})

	when("#Build", func() {
		when("building succeeds", func() {
			it.Before(func() {
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
			})

			it("should ensure the buildpack's layers dir exists and process build layers", func() {
				h.Mkdir(t,
					filepath.Join(layersDir, "A"),
					filepath.Join(appDir, "layers-A-v1", "layer1"),
					filepath.Join(appDir, "layers-A-v1", "layer2"),
					filepath.Join(appDir, "layers-A-v1", "layer3"),
				)
				h.Mkfile(t, "[types]\n  build = true",
					filepath.Join(appDir, "layers-A-v1", "layer1.toml"),
					filepath.Join(appDir, "layers-A-v1", "layer3.toml"),
				)
				// the testdata/buildpack/bin/build script copies the content of the appDir into the layersDir
				gomock.InOrder(
					mockEnv.EXPECT().AddRootDir(filepath.Join(layersDir, "A", "layer1")),
					mockEnv.EXPECT().AddEnvDir(filepath.Join(layersDir, "A", "layer1", "env"), env.ActionTypeOverride),
					mockEnv.EXPECT().AddEnvDir(filepath.Join(layersDir, "A", "layer1", "env.build"), env.ActionTypeOverride),
				)
				gomock.InOrder(
					mockEnv.EXPECT().AddRootDir(filepath.Join(layersDir, "A", "layer3")),
					mockEnv.EXPECT().AddEnvDir(filepath.Join(layersDir, "A", "layer3", "env"), env.ActionTypeOverride),
					mockEnv.EXPECT().AddEnvDir(filepath.Join(layersDir, "A", "layer3", "env.build"), env.ActionTypeOverride),
				)
				if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
				testExists(t,
					filepath.Join(layersDir, "A"),
				)
			})

			it("should provide the platform dir", func() {
				h.Mkfile(t, "some-data",
					filepath.Join(platformDir, "env", "SOME_VAR"),
				)
				if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
				testExists(t,
					filepath.Join(appDir, "build-env-A-v1", "SOME_VAR"),
				)
			})

			it("should provide environment variables", func() {
				if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
				if s := cmp.Diff(h.Rdfile(t, filepath.Join(appDir, "build-info-A-v1")),
					"TEST_ENV: Av1\n",
				); s != "" {
					t.Fatalf("Unexpected info:\n%s\n", s)
				}
			})

			it("should set CNB_BUILDPACK_DIR", func() {
				if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
				if s := cmp.Diff(h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-buildpack-dir-A-v1")),
					bpTOML.Dir,
				); s != "" {
					t.Fatalf("Unexpected CNB_BUILDPACK_DIR:\n%s\n", s)
				}
			})

			it("should set CNB_LAYERS_DIR", func() {
				if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				actual := h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-layers-dir-A-v1"))
				h.AssertEq(t, actual, filepath.Join(layersDir, "A"))
			})

			it("should set CNB_PLATFORM_DIR", func() {
				if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				actual := h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-platform-dir-A-v1"))
				h.AssertEq(t, actual, platformDir)
			})

			it("should set CNB_BP_PLAN_PATH", func() {
				if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				actual := h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-bp-plan-path-A-v1"))
				if actual == "unset" {
					t.Fatal("Expected CNB_BP_PLAN_PATH to be set")
				}
			})

			it("should connect stdout and stdin to the terminal", func() {
				if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
				if s := cmp.Diff(h.CleanEndings(stdout.String()), "build out: A@v1\n"); s != "" {
					t.Fatalf("Unexpected stdout:\n%s\n", s)
				}
				if s := cmp.Diff(h.CleanEndings(stderr.String()), "build err: A@v1\n"); s != "" {
					t.Fatalf("Unexpected stderr:\n%s\n", s)
				}
			})

			when("build result", func() {
				when("met requires", func() {
					it("should derive from build.toml", func() {
						bpPlan := buildpack.Plan{
							Entries: []buildpack.Require{
								{Name: "some-dep"}, {Name: "some-other-dep"}, {Name: "some-unmet-dep"},
							},
						}

						h.Mkfile(t,
							"[[unmet]]\n"+
								`name = "some-unmet-dep"`+"\n",
							filepath.Join(appDir, "build-A-v1.toml"),
						)

						br, err := bpTOML.Build(bpPlan, config, mockEnv)
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}

						if s := cmp.Diff(br, buildpack.BuildResult{
							MetRequires: []string{"some-dep", "some-other-dep"},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})
				})

				when("build bom", func() {
					when("there is a bom in build.toml", func() {
						it("should warn and include the bom", func() {
							h.Mkfile(t,
								"[[bom]]\n"+
									`name = "some-dep"`+"\n"+
									"[bom.metadata]\n"+
									`version = "some-version"`+"\n",
								filepath.Join(appDir, "build-A-v1.toml"),
							)

							br, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
							if err != nil {
								t.Fatalf("Unexpected error:\n%s\n", err)
							}

							if s := cmp.Diff(br, buildpack.BuildResult{
								BuildBOM: []buildpack.BOMEntry{
									{
										Require: buildpack.Require{
											Name:     "some-dep",
											Metadata: map[string]interface{}{"version": "some-version"},
										},
										Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"}, // no api, no homepage
									},
								},
							}); s != "" {
								t.Fatalf("Unexpected:\n%s\n", s)
							}
							assertLogEntry(t, logHandler, "BOM table is deprecated in this buildpack api version, though it remains supported for backwards compatibility. Buildpack authors should write BOM information to <layer>.sbom.<ext>, launch.sbom.<ext>, or build.sbom.<ext>.")
						})
					})

					when("there is a bom in build.toml and SBOM files", func() {
						it("should not warn and should include the bom", func() {
							h.Mkfile(t,
								"[[bom]]\n"+
									`name = "some-dep"`+"\n"+
									"[bom.metadata]\n"+
									`version = "some-version"`+"\n",
								filepath.Join(appDir, "build-A-v1.toml"),
							)

							buildpackID := bpTOML.Buildpack.ID
							bpTOML.Buildpack.SBOM = []string{"application/vnd.cyclonedx+json"}

							h.Mkdir(t,
								filepath.Join(layersDir, buildpackID))
							h.Mkfile(t, `{"key": "some-bom-content"}`,
								filepath.Join(layersDir, buildpackID, "build.sbom.cdx.json"))

							br, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
							if err != nil {
								t.Fatalf("Unexpected error:\n%s\n", err)
							}

							if s := cmp.Diff(br, buildpack.BuildResult{
								BuildBOM: []buildpack.BOMEntry{
									{
										Require: buildpack.Require{
											Name:     "some-dep",
											Metadata: map[string]interface{}{"version": "some-version"},
										},
										Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"}, // no api, no homepage
									},
								},
								BOMFiles: []buildpack.BOMFile{
									{
										BuildpackID: buildpackID,
										LayerName:   "",
										LayerType:   buildpack.LayerTypeBuild,
										Path:        filepath.Join(layersDir, buildpackID, "build.sbom.cdx.json"),
									},
								},
							}); s != "" {
								t.Fatalf("Unexpected:\n%s\n", s)
							}
							assertLogEntryNotContains(t, logHandler, "BOM table is deprecated in this buildpack api version, though it remains supported for backwards compatibility. Buildpack authors should write BOM information to <layer>.sbom.<ext>, launch.sbom.<ext>, or build.sbom.<ext>.")
						})
					})
				})

				when("launch bom", func() {
					when("there is a bom in launch.toml", func() {
						it("should warn and include the bom", func() {
							h.Mkfile(t,
								"[[bom]]\n"+
									`name = "some-dep"`+"\n"+
									"[bom.metadata]\n"+
									`version = "some-version"`+"\n",
								filepath.Join(appDir, "launch-A-v1.toml"),
							)

							br, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
							if err != nil {
								t.Fatalf("Unexpected error:\n%s\n", err)
							}

							if s := cmp.Diff(br, buildpack.BuildResult{
								LaunchBOM: []buildpack.BOMEntry{
									{
										Require: buildpack.Require{
											Name:     "some-dep",
											Metadata: map[string]interface{}{"version": "some-version"},
										},
										Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"}, // no api, no homepage
									},
								},
								Labels:    []buildpack.Label{},
								Processes: []launch.Process{},
								Slices:    []layers.Slice{},
							}); s != "" {
								t.Fatalf("Unexpected:\n%s\n", s)
							}
							assertLogEntry(t, logHandler, "BOM table is deprecated in this buildpack api version, though it remains supported for backwards compatibility. Buildpack authors should write BOM information to <layer>.sbom.<ext>, launch.sbom.<ext>, or build.sbom.<ext>.")
						})
					})

					when("there is a bom in launch.toml and SBOM files", func() {
						it("should not warn and should include the bom", func() {
							h.Mkfile(t,
								"[[bom]]\n"+
									`name = "some-dep"`+"\n"+
									"[bom.metadata]\n"+
									`version = "some-version"`+"\n",
								filepath.Join(appDir, "launch-A-v1.toml"),
							)

							buildpackID := bpTOML.Buildpack.ID
							bpTOML.Buildpack.SBOM = []string{"application/vnd.cyclonedx+json"}

							h.Mkdir(t,
								filepath.Join(layersDir, buildpackID))
							h.Mkfile(t, `{"key": "some-bom-content"}`,
								filepath.Join(layersDir, buildpackID, "launch.sbom.cdx.json"))

							br, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
							if err != nil {
								t.Fatalf("Unexpected error:\n%s\n", err)
							}

							if s := cmp.Diff(br, buildpack.BuildResult{
								LaunchBOM: []buildpack.BOMEntry{
									{
										Require: buildpack.Require{
											Name:     "some-dep",
											Metadata: map[string]interface{}{"version": "some-version"},
										},
										Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"}, // no api, no homepage
									},
								},
								Labels:    []buildpack.Label{},
								Processes: []launch.Process{},
								Slices:    []layers.Slice{},
								BOMFiles: []buildpack.BOMFile{
									{
										BuildpackID: buildpackID,
										LayerName:   "",
										LayerType:   buildpack.LayerTypeLaunch,
										Path:        filepath.Join(layersDir, buildpackID, "launch.sbom.cdx.json"),
									},
								},
							}); s != "" {
								t.Fatalf("Unexpected:\n%s\n", s)
							}
							assertLogEntryNotContains(t, logHandler, "BOM table is deprecated in this buildpack api version, though it remains supported for backwards compatibility. Buildpack authors should write BOM information to <layer>.sbom.<ext>, launch.sbom.<ext>, or build.sbom.<ext>.")
						})
					})

					when("there is a bom in launch.toml with a top-level version", func() {
						it("should error", func() {
							h.Mkfile(t,
								"[[bom]]\n"+
									`name = "some-dep"`+"\n"+
									`version = "some-version"`+"\n",
								filepath.Join(appDir, "launch-A-v1.toml"),
							)

							_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
							h.AssertError(t, err, "bom entry 'some-dep' has a top level version which is not allowed. The buildpack should instead set metadata.version")
						})
					})
				})

				when("bom files", func() {
					it("should include any bom files", func() {
						buildpackID := bpTOML.Buildpack.ID
						bpTOML.Buildpack.SBOM = []string{"application/vnd.cyclonedx+json;version=1.3"}
						layerName := "some-layer"
						otherLayerName := "some-launch-true-cache-false-layer"

						h.Mkdir(t,
							filepath.Join(layersDir, buildpackID))
						h.Mkfile(t, `{"key": "some-bom-content"}`,
							filepath.Join(layersDir, buildpackID, "launch.sbom.cdx.json"),
							filepath.Join(layersDir, buildpackID, "build.sbom.cdx.json"),
							filepath.Join(layersDir, buildpackID, fmt.Sprintf("%s.sbom.cdx.json", layerName)),
							filepath.Join(layersDir, buildpackID, fmt.Sprintf("%s.sbom.cdx.json", otherLayerName)), // layer directory does not exist
						)

						h.Mkdir(t,
							filepath.Join(layersDir, buildpackID, layerName))
						h.Mkfile(t, "[types]\n  cache = true",
							filepath.Join(layersDir, buildpackID, fmt.Sprintf("%s.toml", layerName)))
						h.Mkfile(t, "[types]\n  launch = true\n  cache = false",
							filepath.Join(layersDir, buildpackID, fmt.Sprintf("%s.toml", otherLayerName)))

						br, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
						h.AssertNil(t, err)

						h.AssertEq(t, buildpack.BuildResult{
							BOMFiles: []buildpack.BOMFile{
								{
									BuildpackID: buildpackID,
									LayerName:   "",
									LayerType:   buildpack.LayerTypeBuild,
									Path:        filepath.Join(layersDir, buildpackID, "build.sbom.cdx.json"),
								},
								{
									BuildpackID: buildpackID,
									LayerName:   "",
									LayerType:   buildpack.LayerTypeLaunch,
									Path:        filepath.Join(layersDir, buildpackID, "launch.sbom.cdx.json"),
								},
								{
									BuildpackID: buildpackID,
									LayerName:   otherLayerName,
									LayerType:   buildpack.LayerTypeLaunch,
									Path:        filepath.Join(layersDir, buildpackID, "some-launch-true-cache-false-layer.sbom.cdx.json"),
								},
								{
									BuildpackID: buildpackID,
									LayerName:   layerName,
									LayerType:   buildpack.LayerTypeBuild,
									Path:        filepath.Join(layersDir, buildpackID, "some-layer.sbom.cdx.json"),
								},
								{
									BuildpackID: buildpackID,
									LayerName:   layerName,
									LayerType:   buildpack.LayerTypeCache,
									Path:        filepath.Join(layersDir, buildpackID, "some-layer.sbom.cdx.json"),
								},
							},
						}, br)
					})

					it("should error if there are unsupported extensions", func() {
						buildpackID := bpTOML.Buildpack.ID
						bpTOML.Buildpack.SBOM = []string{"application/vnd.cyclonedx+json", "application/spdx+json", "application/vnd.syft+json"}
						layerName := "some-layer"

						h.Mkdir(t,
							filepath.Join(layersDir, buildpackID, layerName))
						h.Mkfile(t, "[types]\n  launch = true\n  cache = false",
							filepath.Join(layersDir, buildpackID, fmt.Sprintf("%s.toml", layerName)))
						h.Mkfile(t, `{"key": "some-bom-content"}`,
							filepath.Join(layersDir, buildpackID, fmt.Sprintf("%s.sbom.cdx.json", layerName)),
							filepath.Join(layersDir, buildpackID, fmt.Sprintf("%s.sbom.spdx.json", layerName)),
							filepath.Join(layersDir, buildpackID, fmt.Sprintf("%s.sbom.syft.json", layerName)),
							filepath.Join(layersDir, buildpackID, fmt.Sprintf("%s.sbom.some-unknown-format.json", layerName)))

						_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
						h.AssertError(t, err, fmt.Sprintf("unsupported SBOM file format: '%s'", filepath.Join(layersDir, buildpackID, fmt.Sprintf("%s.sbom.some-unknown-format.json", layerName))))
					})

					it("should error if there are undeclared media types", func() {
						buildpackID := bpTOML.Buildpack.ID
						bpTOML.Buildpack.SBOM = []string{"application/vnd.cyclonedx+json"}

						h.Mkdir(t,
							filepath.Join(layersDir, buildpackID))
						h.Mkfile(t, `{"key": "some-bom-content"}`,
							filepath.Join(layersDir, buildpackID, "launch.sbom.spdx.json"))

						_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
						h.AssertError(t, err, fmt.Sprintf("validating SBOM file '%s' for buildpack: 'A@v1': undeclared SBOM media type: 'application/spdx+json'", filepath.Join(layersDir, buildpackID, "launch.sbom.spdx.json")))
					})

					when("buildpack api < 0.7", func() {
						it("should not include any bom files", func() {
							bpTOML.API = api.MustParse("0.6").String()
							buildpackID := bpTOML.Buildpack.ID
							layerName := "some-layer"

							h.Mkdir(t,
								filepath.Join(layersDir, buildpackID))
							h.Mkfile(t, `{"key": "some-bom-content"}`,
								filepath.Join(layersDir, buildpackID, "launch.sbom.cdx.json"),
								filepath.Join(layersDir, buildpackID, "build.sbom.cdx.json"),
								filepath.Join(layersDir, buildpackID, fmt.Sprintf("%s.sbom.cdx.json", layerName)))

							h.Mkdir(t,
								filepath.Join(layersDir, buildpackID, layerName))
							h.Mkfile(t, "[types]\n  cache = true",
								filepath.Join(layersDir, buildpackID, fmt.Sprintf("%s.toml", layerName)))

							br, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
							h.AssertNil(t, err)

							h.AssertEq(t, len(br.BOMFiles), 0)
							expected := "the following SBOM files will be ignored for buildpack api version < 0.7"
							assertLogEntry(t, logHandler, expected)
						})
					})
				})

				when("labels", func() {
					it("should include labels", func() {
						h.Mkfile(t,
							"[[labels]]\n"+
								`key = "some-key"`+"\n"+
								`value = "some-value"`+"\n"+
								"[[labels]]\n"+
								`key = "some-other-key"`+"\n"+
								`value = "some-other-value"`+"\n",
							filepath.Join(appDir, "launch-A-v1.toml"),
						)

						br, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}

						if s := cmp.Diff(br, buildpack.BuildResult{
							LaunchBOM: nil,
							Labels: []buildpack.Label{
								{Key: "some-key", Value: "some-value"},
								{Key: "some-other-key", Value: "some-other-value"},
							},
							MetRequires: nil,
							Processes:   []launch.Process{},
							Slices:      []layers.Slice{},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})
				})

				when("processes", func() {
					it("should include processes and use the default value that is set", func() {
						h.Mkfile(t,
							`[[processes]]`+"\n"+
								`type = "some-type"`+"\n"+
								`command = "some-cmd"`+"\n"+
								`default = true`+"\n"+
								`[[processes]]`+"\n"+
								`type = "web"`+"\n"+
								`command = "other-cmd"`+"\n",
							// default is false and therefore doesn't appear
							filepath.Join(appDir, "launch-A-v1.toml"),
						)
						br, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}
						if s := cmp.Diff(br, buildpack.BuildResult{
							LaunchBOM:   nil,
							Labels:      []buildpack.Label{},
							MetRequires: nil,
							Processes: []launch.Process{
								{Type: "some-type", Command: "some-cmd", BuildpackID: "A", Default: true},
								{Type: "web", Command: "other-cmd", BuildpackID: "A", Default: false},
							},
							Slices: []layers.Slice{},
						}); s != "" {
							t.Fatalf("Unexpected metadata:\n%s\n", s)
						}
					})

					it("should set the working directory", func() {
						h.Mkfile(t,
							"[[processes]]\n"+
								`working-dir = "/working-directory"`,
							filepath.Join(appDir, "launch-A-v1.toml"),
						)
						br, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
						h.AssertNil(t, err)
						h.AssertEq(t, len(br.Processes), 1)
						h.AssertEq(t, br.Processes[0].WorkingDirectory, "/working-directory")
					})
				})

				when("slices", func() {
					it("should include slices", func() {
						h.Mkfile(t,
							"[[slices]]\n"+
								`paths = ["some-path", "some-other-path"]`+"\n",
							filepath.Join(appDir, "launch-A-v1.toml"),
						)

						br, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}

						if s := cmp.Diff(br, buildpack.BuildResult{
							LaunchBOM:   nil,
							Labels:      []buildpack.Label{},
							MetRequires: nil,
							Processes:   []launch.Process{},
							Slices:      []layers.Slice{{Paths: []string{"some-path", "some-other-path"}}},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})
				})
			})

			when("the launch, cache and build flags are false", func() {
				when("the flags are specified in <layer>.toml", func() {
					it("renames <layers>/<layer> to <layers>/<layer>.ignore", func() {
						h.Mkdir(t,
							filepath.Join(layersDir, "A", "layer"),
						)
						h.Mkfile(t,
							"[types]\n  build=false\n  cache=false\n  launch=false",
							filepath.Join(layersDir, "A", "layer.toml"),
						)

						_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
						h.AssertNil(t, err)
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "A", "layer"))
						h.AssertPathExists(t, filepath.Join(layersDir, "A", "layer.ignore"))
					})
				})

				when("the flags aren't specified in <layer>.toml", func() {
					it("renames <layers>/<layer> to <layers>/<layer>.ignore", func() {
						h.Mkdir(t,
							filepath.Join(layersDir, "A", "layer"),
						)
						h.Mkfile(t,
							"",
							filepath.Join(layersDir, "A", "layer.toml"),
						)

						_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
						h.AssertNil(t, err)
						h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "A", "layer"))
						h.AssertPathExists(t, filepath.Join(layersDir, "A", "layer.ignore"))
					})
				})
			})
		})

		when("building succeeds with a clear env", func() {
			it.Before(func() {
				mockEnv.EXPECT().List().Return(append(os.Environ(), "TEST_ENV=cleared"))

				bpTOML.Buildpack.Version = "v1.clear"
				bpTOML.Dir = filepath.Join(buildpacksDir, "A", "v1.clear")
				bpTOML.Buildpack.ClearEnv = true
			})

			it("should not apply user-provided env vars", func() {
				if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				if s := cmp.Diff(h.Rdfile(t, filepath.Join(appDir, "build-info-A-v1.clear")),
					"TEST_ENV: cleared\n",
				); s != "" {
					t.Fatalf("Unexpected info:\n%s\n", s)
				}
			})

			it("should set CNB_BUILDPACK_DIR", func() {
				if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				if s := cmp.Diff(h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-buildpack-dir-A-v1.clear")),
					bpTOML.Dir,
				); s != "" {
					t.Fatalf("Unexpected CNB_BUILDPACK_DIR:\n%s\n", s)
				}
			})
		})

		when("building fails", func() {
			it("should error when layer directories cannot be created", func() {
				h.Mkfile(t, "some-data", filepath.Join(layersDir, "A"))
				_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
				if _, ok := err.(*os.PathError); !ok {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			it("should error when the provided buildpack plan is invalid", func() {
				bpPlan := buildpack.Plan{
					Entries: []buildpack.Require{
						{
							Metadata: map[string]interface{}{"a": map[int64]int64{1: 2}}, // map with non-string key type
						},
					},
				}
				if _, err := bpTOML.Build(bpPlan, config, mockEnv); err == nil {
					t.Fatal("Expected error.\n")
				} else if !strings.Contains(err.Error(), "toml") {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			it("should error when the env cannot be found", func() {
				mockEnv.EXPECT().WithPlatform(platformDir).Return(nil, errors.New("some error"))
				if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err == nil {
					t.Fatal("Expected error.\n")
				} else if !strings.Contains(err.Error(), "some error") {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			it("should error when the command fails", func() {
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
				if err := os.RemoveAll(platformDir); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
				if err, ok := err.(*buildpack.Error); !ok || err.Type != buildpack.ErrTypeBuildpack {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			when("modifying the env fails", func() {
				var appendErr error

				it.Before(func() {
					appendErr = errors.New("some error")
				})

				each(it, []func(){
					func() {
						mockEnv.EXPECT().AddRootDir(gomock.Any()).Return(appendErr)
					},
					func() {
						mockEnv.EXPECT().AddRootDir(gomock.Any()).Return(nil)
						mockEnv.EXPECT().AddEnvDir(gomock.Any(), gomock.Any()).Return(appendErr)
					},
					func() {
						mockEnv.EXPECT().AddRootDir(gomock.Any()).Return(nil)
						mockEnv.EXPECT().AddEnvDir(gomock.Any(), gomock.Any()).Return(nil)
						mockEnv.EXPECT().AddEnvDir(gomock.Any(), gomock.Any()).Return(nil)
						mockEnv.EXPECT().AddRootDir(gomock.Any()).Return(appendErr)
					},
					func() {
						mockEnv.EXPECT().AddRootDir(gomock.Any()).Return(nil)
						mockEnv.EXPECT().AddEnvDir(gomock.Any(), gomock.Any()).Return(nil)
						mockEnv.EXPECT().AddEnvDir(gomock.Any(), gomock.Any()).Return(nil)
						mockEnv.EXPECT().AddRootDir(gomock.Any()).Return(nil)
						mockEnv.EXPECT().AddEnvDir(gomock.Any(), gomock.Any()).Return(nil)
						mockEnv.EXPECT().AddEnvDir(gomock.Any(), gomock.Any()).Return(appendErr)
					},
				}, "should error", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
					h.Mkdir(t,
						filepath.Join(appDir, "layers-A-v1", "layer1"),
						filepath.Join(appDir, "layers-A-v1", "layer2"),
					)
					h.Mkfile(t, "[types]\n  build = true",
						filepath.Join(appDir, "layers-A-v1", "layer1.toml"),
						filepath.Join(appDir, "layers-A-v1", "layer2.toml"),
					)
					if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err != appendErr {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})
			})

			when("invalid unmet entries", func() {
				when("missing name", func() {
					it("should error", func() {
						mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
						h.Mkfile(t,
							"[[unmet]]\n",
							filepath.Join(appDir, "build-A-v1.toml"),
						)
						_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
						h.AssertNotNil(t, err)
						expected := "name is required"
						h.AssertStringContains(t, err.Error(), expected)
					})
				})

				when("invalid name", func() {
					it("should error", func() {
						mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
						h.Mkfile(t,
							"[[unmet]]\n"+
								`name = "unknown-dep"`+"\n",
							filepath.Join(appDir, "build-A-v1.toml"),
						)
						_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
						h.AssertNotNil(t, err)
						expected := "must match a requested dependency"
						h.AssertStringContains(t, err.Error(), expected)
					})
				})
			})

			when("there is more than one default=true process", func() {
				it.Before(func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
				})

				when("the processes are with the same type", func() {
					it("should error", func() {
						h.Mkfile(t,
							`[[processes]]`+"\n"+
								`type = "some-type"`+"\n"+
								`command = "some-cmd"`+"\n"+
								`default = true`+"\n"+
								`[[processes]]`+"\n"+
								`type = "some-type"`+"\n"+
								`command = "some-other-cmd"`+"\n"+
								`default = true`+"\n",
							filepath.Join(appDir, "launch-A-v1.toml"),
						)
						_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
						h.AssertNotNil(t, err)
						expected := "multiple default process types aren't allowed"
						h.AssertStringContains(t, err.Error(), expected)
					})
				})

				when("the processes are with different types", func() {
					it("should error", func() {
						h.Mkfile(t,
							`[[processes]]`+"\n"+
								`type = "some-type"`+"\n"+
								`command = "some-cmd"`+"\n"+
								`default = true`+"\n"+
								`[[processes]]`+"\n"+
								`type = "other-type"`+"\n"+
								`command = "other-cmd"`+"\n"+
								`default = true`+"\n",
							filepath.Join(appDir, "launch-A-v1.toml"),
						)
						_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
						h.AssertNotNil(t, err)
						expected := "multiple default process types aren't allowed"
						h.AssertStringContains(t, err.Error(), expected)
					})
				})
			})

			when("the launch, cache and build flags are in the top level", func() {
				it("should error", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
					h.Mkdir(t,
						filepath.Join(layersDir, "A"),
						filepath.Join(appDir, "layers-A-v1", "layer"),
					)
					h.Mkfile(t,
						"build=true\ncache=true\nlaunch=true",
						filepath.Join(appDir, "layers-A-v1", "layer.toml"),
					)

					_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
					h.AssertNotNil(t, err)
					expected := "the launch, cache and build flags should be in the types table"
					h.AssertStringContains(t, err.Error(), expected)
				})
			})
		})

		when("buildpack api = 0.2", func() {
			it.Before(func() {
				bpTOML.API = "0.2"
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
			})

			it("should convert metadata version to top level version in the buildpack plan", func() {
				bpPlan := buildpack.Plan{
					Entries: []buildpack.Require{
						{
							Name:     "some-dep",
							Metadata: map[string]interface{}{"version": "v1"},
						},
					},
				}

				_, err := bpTOML.Build(bpPlan, config, mockEnv)
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				testPlan(t,
					[]buildpack.Require{
						{
							Name:     "some-dep",
							Version:  "v1",
							Metadata: map[string]interface{}{"version": "v1"},
						},
					},
					filepath.Join(appDir, "build-plan-in-A-v1.toml"),
				)
			})
		})

		when("buildpack api < 0.5", func() {
			it.Before(func() {
				bpTOML.API = "0.4"
			})

			when("building succeeds", func() {
				it.Before(func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
				})

				it("should ensure the buildpack's layers dir exists and process build layers", func() {
					h.Mkdir(t,
						filepath.Join(layersDir, "A"),
						filepath.Join(appDir, "layers-A-v1", "layer1"),
						filepath.Join(appDir, "layers-A-v1", "layer2"),
						filepath.Join(appDir, "layers-A-v1", "layer3"),
					)
					h.Mkfile(t, "build = true",
						filepath.Join(appDir, "layers-A-v1", "layer1.toml"),
						filepath.Join(appDir, "layers-A-v1", "layer3.toml"),
					)
					gomock.InOrder(
						mockEnv.EXPECT().AddRootDir(filepath.Join(layersDir, "A", "layer1")),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(layersDir, "A", "layer1", "env"), env.ActionTypePrependPath),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(layersDir, "A", "layer1", "env.build"), env.ActionTypePrependPath),
					)
					gomock.InOrder(
						mockEnv.EXPECT().AddRootDir(filepath.Join(layersDir, "A", "layer3")),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(layersDir, "A", "layer3", "env"), env.ActionTypePrependPath),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(layersDir, "A", "layer3", "env.build"), env.ActionTypePrependPath),
					)
					if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					testExists(t,
						filepath.Join(layersDir, "A"),
					)
				})

				it("should get bom entries and unmet requires from the output buildpack plan", func() {
					bpPlan := buildpack.Plan{
						Entries: []buildpack.Require{
							{
								Name:    "some-deprecated-bp-dep",
								Version: "v1", // top-level version is deprecated in buildpack API 0.3
							},
							{
								Name:    "some-deprecated-bp-replace-version-dep",
								Version: "some-version-orig", // top-level version is deprecated in buildpack API 0.3
							},
							{
								Name:     "some-dep",
								Metadata: map[string]interface{}{"version": "v1"},
							},
							{
								Name:     "some-replace-version-dep",
								Metadata: map[string]interface{}{"version": "some-version-orig"},
							},
							{
								Name: "some-unmet-dep",
							},
						},
					}

					h.Mkfile(t,
						"[[entries]]\n"+
							`name = "some-deprecated-bp-dep"`+"\n"+
							`version = "v1"`+"\n"+
							"[[entries]]\n"+
							`name = "some-deprecated-bp-replace-version-dep"`+"\n"+
							`version = "some-version-new"`+"\n"+
							"[[entries]]\n"+
							`name = "some-dep"`+"\n"+
							"[entries.metadata]\n"+
							`version = "v1"`+"\n"+
							"[[entries]]\n"+
							`name = "some-replace-version-dep"`+"\n"+
							"[entries.metadata]\n"+
							`version = "some-version-new"`+"\n",
						filepath.Join(appDir, "build-plan-out-A-v1.toml"),
					)

					br, err := bpTOML.Build(bpPlan, config, mockEnv)
					if err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}

					if s := cmp.Diff(br, buildpack.BuildResult{
						LaunchBOM: []buildpack.BOMEntry{
							{
								Require: buildpack.Require{
									Name:     "some-deprecated-bp-dep",
									Metadata: map[string]interface{}{"version": "v1"},
								},
								Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"},
							},
							{
								Require: buildpack.Require{
									Name:     "some-deprecated-bp-replace-version-dep",
									Metadata: map[string]interface{}{"version": "some-version-new"},
								},
								Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"},
							},
							{
								Require: buildpack.Require{
									Name:     "some-dep",
									Metadata: map[string]interface{}{"version": "v1"},
								},
								Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"},
							},
							{
								Require: buildpack.Require{
									Name:     "some-replace-version-dep",
									Metadata: map[string]interface{}{"version": "some-version-new"},
								},
								Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"},
							},
						},
						Labels: nil,
						MetRequires: []string{
							"some-deprecated-bp-dep",
							"some-deprecated-bp-replace-version-dep",
							"some-dep",
							"some-replace-version-dep",
						},
						Processes: nil,
						Slices:    nil,
					}); s != "" {
						t.Fatalf("Unexpected:\n%s\n", s)
					}
				})

				it("should convert top level version to metadata.version in the bom", func() {
					h.Mkfile(t,
						"[[entries]]\n"+
							`name = "dep-1"`+"\n"+
							`version = "v1"`+"\n"+
							"[[entries]]\n"+
							`name = "dep-2"`+"\n"+
							`version = "v2"`+"\n"+
							"[entries.metadata]\n"+
							`version = "v2"`+"\n"+
							"[[entries]]\n"+
							`name = "dep-3"`+"\n"+
							"[entries.metadata]\n"+
							`version = "v3"`+"\n",
						filepath.Join(appDir, "build-plan-out-A-v1.toml"),
					)

					br, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
					if err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}

					if s := cmp.Diff(br.LaunchBOM, []buildpack.BOMEntry{
						{
							Require: buildpack.Require{
								Name:     "dep-1",
								Metadata: map[string]interface{}{"version": "v1"},
							},
							Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"},
						},
						{
							Require: buildpack.Require{
								Name:     "dep-2",
								Metadata: map[string]interface{}{"version": "v2"},
							},
							Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"},
						},
						{
							Require: buildpack.Require{
								Name:     "dep-3",
								Metadata: map[string]interface{}{"version": "v3"},
							},
							Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"},
						},
					}); s != "" {
						t.Fatalf("Unexpected:\n%s\n", s)
					}
				})
			})

			when("building fails", func() {
				it("should error when the output buildpack plan is invalid", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
					h.Mkfile(t, "bad-key", filepath.Join(appDir, "build-plan-out-A-v1.toml"))
					if _, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv); err == nil {
						t.Fatal("Expected error.\n")
					} else if !strings.Contains(err.Error(), "key") {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})

				it("should error when top level version and metadata version are both present and do not match", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
					h.Mkfile(t,
						"[[entries]]\n"+
							`name = "dep1"`+"\n"+
							`version = "v2"`+"\n"+
							"[entries.metadata]\n"+
							`version = "v1"`+"\n",
						filepath.Join(appDir, "build-plan-out-A-v1.toml"),
					)
					_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
					h.AssertNotNil(t, err)
					expected := "top level version does not match metadata version"
					h.AssertStringContains(t, err.Error(), expected)
				})
			})
		})

		when("buildpack api < 0.6", func() {
			it.Before(func() {
				bpTOML.API = "0.5"
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
			})

			it("should include processes and set/override their default value to false", func() {
				h.Mkfile(t,
					`[[processes]]`+"\n"+
						`type = "type-with-no-default"`+"\n"+
						`command = "some-cmd"`+"\n"+
						`[[processes]]`+"\n"+
						`type = "type-with-default"`+"\n"+
						`command = "other-cmd"`+"\n"+
						`default = true`+"\n",
					filepath.Join(appDir, "launch-A-v1.toml"),
				)
				br, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
				if s := cmp.Diff(br, buildpack.BuildResult{
					LaunchBOM:   nil,
					Labels:      []buildpack.Label{},
					MetRequires: nil,
					Processes: []launch.Process{
						{Type: "type-with-no-default", Command: "some-cmd", BuildpackID: "A", Default: false},
						{Type: "type-with-default", Command: "other-cmd", BuildpackID: "A", Default: false},
					},
					Slices: []layers.Slice{},
				}); s != "" {
					t.Fatalf("Unexpected metadata:\n%s\n", s)
				}
				expected := "Warning: default processes aren't supported in this buildpack api version. Overriding the default value to false for the following processes: [type-with-default]"
				assertLogEntry(t, logHandler, expected)
			})

			when("the launch, cache and build flags are in the types table", func() {
				it("should warn", func() {
					h.Mkdir(t,
						filepath.Join(layersDir, "A"),
						filepath.Join(appDir, "layers-A-v1", "layer"),
					)
					h.Mkfile(t,
						"[types]\n  build=true\n  cache=true\n  launch=true",
						filepath.Join(appDir, "layers-A-v1", "layer.toml"),
					)

					_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
					h.AssertNil(t, err)
					expected := "Types table isn't supported in this buildpack api version. The launch, build and cache flags should be in the top level. Ignoring the values in the types table."
					assertLogEntry(t, logHandler, expected)
				})
			})
		})

		when("buildpack api < 0.7", func() {
			it.Before(func() {
				bpTOML.API = "0.6"
			})

			it("should get bom entries from launch.toml and unmet requires from build.toml", func() {
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)

				bpPlan := buildpack.Plan{
					Entries: []buildpack.Require{
						{
							Name:    "some-deprecated-bp-replace-version-dep",
							Version: "some-version-orig", // top-level version is deprecated in buildpack API 0.3
						},
						{
							Name:     "some-dep",
							Metadata: map[string]interface{}{"version": "v1"},
						},
						{
							Name:     "some-replace-version-dep",
							Metadata: map[string]interface{}{"version": "some-version-orig"},
						},
						{
							Name: "some-unmet-dep",
						},
					},
				}

				h.Mkfile(t,
					"[[bom]]\n"+
						`name = "some-deprecated-bp-replace-version-dep"`+"\n"+
						"[bom.metadata]\n"+
						`version = "some-version-new"`+"\n"+
						"[[bom]]\n"+
						`name = "some-dep"`+"\n"+
						"[bom.metadata]\n"+
						`version = "v1"`+"\n"+
						"[[bom]]\n"+
						`name = "some-replace-version-dep"`+"\n"+
						"[bom.metadata]\n"+
						`version = "some-version-new"`+"\n",
					filepath.Join(appDir, "launch-A-v1.toml"),
				)

				h.Mkfile(t,
					"[[unmet]]\n"+
						`name = "some-unmet-dep"`+"\n",
					filepath.Join(appDir, "build-A-v1.toml"),
				)

				br, err := bpTOML.Build(bpPlan, config, mockEnv)
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := cmp.Diff(br, buildpack.BuildResult{
					LaunchBOM: []buildpack.BOMEntry{
						{
							Require: buildpack.Require{
								Name:     "some-deprecated-bp-replace-version-dep",
								Metadata: map[string]interface{}{"version": "some-version-new"},
							},
							Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"}, // no api, no homepage
						},
						{
							Require: buildpack.Require{
								Name:     "some-dep",
								Metadata: map[string]interface{}{"version": "v1"},
							},
							Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"}, // no api, no homepage
						},
						{
							Require: buildpack.Require{
								Name:     "some-replace-version-dep",
								Metadata: map[string]interface{}{"version": "some-version-new"},
							},
							Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"}, // no api, no homepage
						},
					},
					Labels:      []buildpack.Label{},
					MetRequires: []string{"some-deprecated-bp-replace-version-dep", "some-dep", "some-replace-version-dep"},
					Processes:   []launch.Process{},
					Slices:      []layers.Slice{},
				}); s != "" {
					t.Fatalf("Unexpected:\n%s\n", s)
				}
			})

			it("should error when the launch bom has a top level version", func() {
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
				h.Mkfile(t,
					"[[bom]]\n"+
						`name = "some-dep"`+"\n"+
						`version = "some-version"`+"\n",
					filepath.Join(appDir, "launch-A-v1.toml"),
				)
				_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
				h.AssertNotNil(t, err)
				expected := "top level version which is not allowed"
				h.AssertStringContains(t, err.Error(), expected)
			})

			it("should error when the build bom has a top level version", func() {
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
				h.Mkfile(t,
					"[[bom]]\n"+
						`name = "some-dep"`+"\n"+
						`version = "some-version"`+"\n",
					filepath.Join(appDir, "build-A-v1.toml"),
				)
				_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
				h.AssertNotNil(t, err)
				expected := "top level version which is not allowed"
				h.AssertStringContains(t, err.Error(), expected)
			})
		})

		when("buildpack api < 0.8", func() {
			it.Before(func() {
				bpTOML.API = "0.7"
			})

			it("should not set environment variables for positional arguments", func() {
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)

				_, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)

				h.AssertNil(t, err)
				for _, file := range []string{
					"build-env-cnb-layers-dir-A-v1",
					"build-env-cnb-platform-dir-A-v1",
					"build-env-cnb-bp-plan-path-A-v1",
				} {
					contents := h.Rdfile(t, filepath.Join(appDir, file))
					if contents != "unset" {
						t.Fatalf("Expected %s to be unset; got %s", file, contents)
					}
				}
			})

			it("should ignore process working directory and warn", func() {
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
				h.Mkfile(t,
					"[[processes]]\n"+
						`working-dir = "/working-directory"`+"\n"+
						`type = "some-type"`+"\n",
					filepath.Join(appDir, "launch-A-v1.toml"),
				)
				br, err := bpTOML.Build(buildpack.Plan{}, config, mockEnv)
				h.AssertNil(t, err)
				h.AssertEq(t, len(br.Processes), 1)
				h.AssertEq(t, br.Processes[0].WorkingDirectory, "")
				assertLogEntry(t, logHandler, "Warning: process working directory isn't supported in this buildpack api version. Ignoring working directory for process 'some-type'")
			})
		})
	})
}

func testExists(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func testPlan(t *testing.T, plan []buildpack.Require, paths ...string) {
	t.Helper()
	for _, p := range paths {
		var c struct {
			Entries []buildpack.Require `toml:"entries"`
		}
		if _, err := toml.DecodeFile(p, &c); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		if s := cmp.Diff(c.Entries, plan); s != "" {
			t.Fatalf("Unexpected plan:\n%s\n", s)
		}
	}
}

func each(it spec.S, befores []func(), text string, f func()) {
	for i := range befores {
		before := befores[i]
		it(fmt.Sprintf("%s #%d", text, i), func() { before(); f() })
	}
}

func assertLogEntry(t *testing.T, logHandler *memory.Handler, expected string) {
	t.Helper()
	var messages []string
	for _, le := range logHandler.Entries {
		messages = append(messages, le.Message)
		if strings.Contains(le.Message, expected) {
			return
		}
	}
	fmtMessage := "\n" + strings.Join(messages, "\n") + "\n"
	t.Fatalf("Expected log entries: %s to contain \n'%s'", fmtMessage, expected)
}

func assertLogEntryNotContains(t *testing.T, logHandler *memory.Handler, expected string) {
	t.Helper()
	var messages []string
	for _, le := range logHandler.Entries {
		messages = append(messages, le.Message)
		if strings.Contains(le.Message, expected) {
			fmtMessage := "\n" + strings.Join(messages, "\n") + "\n"
			t.Fatalf("Expected log entries: %s not to contain \n'%s'", fmtMessage, expected)
		}
	}
}
