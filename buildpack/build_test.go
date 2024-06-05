package buildpack_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
	llog "github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/phase/testmock"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

//go:generate mockgen -package testmock -destination testmock/env.go github.com/buildpacks/lifecycle/phase BuildEnv

func TestBuild(t *testing.T) {
	spec.Run(t, "unit-build", testBuild, spec.Report(report.Terminal{}))
}

// PlatformAPI should be ignored because it isn't set this early in the lifecycle
var processCmpOpts = []cmp.Option{
	cmpopts.IgnoreFields(launch.Process{}, "PlatformAPI"),
	cmpopts.IgnoreFields(launch.RawCommand{}, "PlatformAPI"),
}

func testBuild(t *testing.T, when spec.G, it spec.S) {
	var (
		mockCtrl   *gomock.Controller
		executor   *buildpack.DefaultBuildExecutor
		dirStore   string
		descriptor buildpack.BpDescriptor

		// build inputs
		inputs         buildpack.BuildInputs
		tmpDir         string
		appDir         string
		buildConfigDir string
		layersDir      string
		platformDir    string
		mockEnv        *testmock.MockBuildEnv
		stdout, stderr *bytes.Buffer

		logger     llog.Logger
		logHandler = memory.New()
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		executor = &buildpack.DefaultBuildExecutor{}

		// setup descriptor
		var err error
		dirStore, err = filepath.Abs(filepath.Join("testdata", "buildpack", "by-id"))
		h.AssertNil(t, err)
		descriptor = buildpack.BpDescriptor{
			WithAPI: api.Buildpack.Latest().String(),
			Buildpack: buildpack.BpInfo{
				BaseInfo: buildpack.BaseInfo{
					ID:       "A",
					Version:  "v1",
					Name:     "Buildpack A",
					ClearEnv: false,
					Homepage: "Buildpack A Homepage",
				},
			},
			WithRootDir: filepath.Join(dirStore, "A", "v1"),
		}

		// setup dirs
		tmpDir, err = os.MkdirTemp("", "lifecycle")
		h.AssertNil(t, err)
		layersDir = filepath.Join(tmpDir, "launch")
		appDir = filepath.Join(layersDir, "app")
		buildConfigDir = filepath.Join(tmpDir, "build-config")
		platformDir = filepath.Join(tmpDir, "platform")
		h.Mkdir(t, layersDir, appDir, filepath.Join(platformDir, "env"))

		// make inputs
		mockEnv = testmock.NewMockBuildEnv(mockCtrl)
		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
		inputs = buildpack.BuildInputs{
			AppDir:         appDir,
			BuildConfigDir: buildConfigDir,
			LayersDir:      layersDir,
			PlatformDir:    platformDir,
			Env:            mockEnv,
			Out:            stdout,
			Err:            stderr,
		}
		logger = &log.Logger{Handler: logHandler}
	})

	it.After(func() {
		_ = os.RemoveAll(tmpDir)
		mockCtrl.Finish()
	})

	when("#Build", func() {
		when("env", func() {
			when("clear", func() {
				it.Before(func() {
					mockEnv.EXPECT().WithOverrides("", buildConfigDir).Return(append(os.Environ(), "TEST_ENV=cleared"), nil)

					descriptor.Buildpack.Version = "v1.clear"
					descriptor.WithRootDir = filepath.Join(dirStore, "A", "v1.clear")
					descriptor.Buildpack.ClearEnv = true
				})

				it("provides a clear env", func() {
					if _, err := executor.Build(descriptor, inputs, logger); err != nil {
						t.Fatalf("Error: %s\n", err)
					}
					if s := cmp.Diff(h.Rdfile(t, filepath.Join(appDir, "build-info-A-v1.clear")),
						"TEST_ENV: cleared\n",
					); s != "" {
						t.Fatalf("Unexpected info:\n%s\n", s)
					}
				})

				it("sets CNB_ vars", func() {
					if _, err := executor.Build(descriptor, inputs, logger); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}

					var actual string
					t.Log("sets CNB_BUILDPACK_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-buildpack-dir-A-v1.clear"))
					h.AssertEq(t, actual, descriptor.WithRootDir)

					t.Log("sets CNB_PLATFORM_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-platform-dir-A-v1.clear"))
					h.AssertEq(t, actual, platformDir)

					t.Log("sets CNB_BP_PLAN_PATH")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-bp-plan-path-A-v1.clear"))
					if isUnset(actual) {
						t.Fatal("Expected CNB_BP_PLAN_PATH to be set")
					}

					t.Log("sets CNB_LAYERS_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-layers-dir-A-v1.clear"))
					h.AssertEq(t, actual, filepath.Join(layersDir, "A"))
					t.Log("does not set CNB_OUTPUT_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-output-dir-A-v1.clear"))
					h.AssertEq(t, isUnset(actual), true)
				})
			})

			when("full", func() {
				it.Before(func() {
					mockEnv.EXPECT().WithOverrides(platformDir, buildConfigDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
				})

				it("provides a full env", func() {
					if _, err := executor.Build(descriptor, inputs, logger); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := cmp.Diff(h.Rdfile(t, filepath.Join(appDir, "build-info-A-v1")),
						"TEST_ENV: Av1\n",
					); s != "" {
						t.Fatalf("Unexpected info:\n%s\n", s)
					}
				})

				it("sets CNB_ vars", func() {
					if _, err := executor.Build(descriptor, inputs, logger); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}

					var actual string
					t.Log("sets CNB_BUILDPACK_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-buildpack-dir-A-v1"))
					h.AssertEq(t, actual, descriptor.WithRootDir)

					t.Log("sets CNB_PLATFORM_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-platform-dir-A-v1"))
					h.AssertEq(t, actual, platformDir)

					t.Log("sets CNB_BP_PLAN_PATH")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-bp-plan-path-A-v1"))
					if isUnset(actual) {
						t.Fatal("Expected CNB_BP_PLAN_PATH to be set")
					}

					t.Log("sets CNB_LAYERS_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-layers-dir-A-v1"))
					h.AssertEq(t, actual, filepath.Join(layersDir, "A"))
					t.Log("does not set CNB_OUTPUT_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-output-dir-A-v1"))
					h.AssertEq(t, isUnset(actual), true)
				})

				it("loads env vars from <platform>/env", func() {
					h.Mkfile(t, "some-data",
						filepath.Join(platformDir, "env", "SOME_VAR"),
					)
					if _, err := executor.Build(descriptor, inputs, logger); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					testExists(t,
						filepath.Join(appDir, "build-env-A-v1", "SOME_VAR"),
					)
				})
			})

			it("errors when <platform>/env cannot be loaded", func() {
				mockEnv.EXPECT().WithOverrides(platformDir, buildConfigDir).Return(nil, errors.New("some error"))
				if _, err := executor.Build(descriptor, inputs, logger); err == nil {
					t.Fatal("Expected error.\n")
				} else if !strings.Contains(err.Error(), "some error") {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			when("any", func() {
				it.Before(func() {
					mockEnv.EXPECT().WithOverrides(platformDir, buildConfigDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil).AnyTimes()
				})

				it("ensures the buildpack's layers dir exists and processes build layers", func() {
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
					if _, err := executor.Build(descriptor, inputs, logger); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					testExists(t,
						filepath.Join(layersDir, "A"),
					)
				})

				it("errors when the buildpack's layers dir cannot be created", func() {
					h.Mkfile(t, "some-data", filepath.Join(layersDir, "A"))
					_, err := executor.Build(descriptor, inputs, logger)
					if _, ok := err.(*os.PathError); !ok {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})

				it("errors when the provided buildpack plan is invalid", func() {
					inputs.Plan = buildpack.Plan{
						Entries: []buildpack.Require{
							{
								Metadata: map[string]interface{}{"a": map[int64]int64{1: 2}}, // map with non-string key type
							},
						},
					}
					if _, err := executor.Build(descriptor, inputs, logger); err == nil {
						t.Fatal("Expected error.\n")
					} else if !strings.Contains(err.Error(), "toml") {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})

				it("connects stdout and stdin to the terminal", func() {
					if _, err := executor.Build(descriptor, inputs, logger); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := cmp.Diff(h.CleanEndings(stdout.String()), "build out: A@v1\n"); s != "" {
						t.Fatalf("Unexpected stdout:\n%s\n", s)
					}
					if s := cmp.Diff(h.CleanEndings(stderr.String()), "build err: A@v1\n"); s != "" {
						t.Fatalf("Unexpected stderr:\n%s\n", s)
					}
				})

				when("modifying the env fails", func() {
					var appendErr error

					it.Before(func() {
						mockEnv.EXPECT().WithOverrides(platformDir, buildConfigDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil).AnyTimes()
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
					}, "errors", func() {
						h.Mkdir(t,
							filepath.Join(appDir, "layers-A-v1", "layer1"),
							filepath.Join(appDir, "layers-A-v1", "layer2"),
						)
						h.Mkfile(t, "[types]\n  build = true",
							filepath.Join(appDir, "layers-A-v1", "layer1.toml"),
							filepath.Join(appDir, "layers-A-v1", "layer2.toml"),
						)
						if _, err := executor.Build(descriptor, inputs, logger); err != appendErr {
							t.Fatalf("Incorrect error: %s\n", err)
						}
					})
				})

				it("errors when the command fails", func() {
					if err := os.RemoveAll(platformDir); err != nil {
						t.Fatalf("Error: %s\n", err)
					}
					_, err := executor.Build(descriptor, inputs, logger)
					if err, ok := err.(*buildpack.Error); !ok || err.Type != buildpack.ErrTypeBuildpack {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})

				when("<layer>.toml", func() {
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

								_, err := executor.Build(descriptor, inputs, logger)
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

								_, err := executor.Build(descriptor, inputs, logger)
								h.AssertNil(t, err)
								h.AssertPathDoesNotExist(t, filepath.Join(layersDir, "A", "layer"))
								h.AssertPathExists(t, filepath.Join(layersDir, "A", "layer.ignore"))
							})
						})
					})

					it("errors when the launch, cache and build flags are in the top level", func() {
						h.Mkdir(t,
							filepath.Join(layersDir, "A"),
							filepath.Join(appDir, "layers-A-v1", "layer"),
						)
						h.Mkfile(t,
							"build=true\ncache=true\nlaunch=true",
							filepath.Join(appDir, "layers-A-v1", "layer.toml"),
						)

						_, err := executor.Build(descriptor, inputs, logger)
						h.AssertNotNil(t, err)
						expected := "the launch, cache and build flags should be in the types table"
						h.AssertStringContains(t, err.Error(), expected)
					})
				})

				when("build result", func() {
					when("build bom", func() {
						when("there is a bom in build.toml", func() {
							it("warns and includes the bom", func() {
								h.Mkfile(t,
									"[[bom]]\n"+
										`name = "some-dep"`+"\n"+
										"[bom.metadata]\n"+
										`version = "some-version"`+"\n",
									filepath.Join(appDir, "build-A-v1.toml"),
								)

								br, err := executor.Build(descriptor, inputs, logger)
								h.AssertNil(t, err)

								h.AssertEq(t, br.BuildBOM, []buildpack.BOMEntry{
									{
										Require: buildpack.Require{
											Name:     "some-dep",
											Metadata: map[string]interface{}{"version": "some-version"},
										},
										Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"}, // no api, no homepage
									},
								})
								assertLogEntry(t, logHandler, "BOM table is deprecated in this buildpack api version, though it remains supported for backwards compatibility. Buildpack authors should write BOM information to <layer>.sbom.<ext>, launch.sbom.<ext>, or build.sbom.<ext>.")
							})
						})

						when("there is a bom in build.toml and SBOM files", func() {
							it("does not warn and does not include the bom", func() {
								h.Mkfile(t,
									"[[bom]]\n"+
										`name = "some-dep"`+"\n"+
										"[bom.metadata]\n"+
										`version = "some-version"`+"\n",
									filepath.Join(appDir, "build-A-v1.toml"),
								)

								buildpackID := descriptor.Buildpack.ID
								descriptor.Buildpack.SBOM = []string{"application/vnd.cyclonedx+json"}

								h.Mkdir(t,
									filepath.Join(layersDir, buildpackID))
								h.Mkfile(t, `{"key": "some-bom-content"}`,
									filepath.Join(layersDir, buildpackID, "build.sbom.cdx.json"))

								br, err := executor.Build(descriptor, inputs, logger)
								h.AssertNil(t, err)

								h.AssertEq(t, br.BuildBOM, []buildpack.BOMEntry{
									{
										Require: buildpack.Require{
											Name:     "some-dep",
											Metadata: map[string]interface{}{"version": "some-version"},
										},
										Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"}, // no api, no homepage
									},
								})
								h.AssertEq(t, br.BOMFiles, []buildpack.BOMFile{
									{
										BuildpackID: buildpackID,
										LayerName:   "",
										LayerType:   buildpack.LayerTypeBuild,
										Path:        filepath.Join(layersDir, buildpackID, "build.sbom.cdx.json"),
									},
								})
								assertLogEntryNotContains(t, logHandler, "BOM table is deprecated in this buildpack api version, though it remains supported for backwards compatibility. Buildpack authors should write BOM information to <layer>.sbom.<ext>, launch.sbom.<ext>, or build.sbom.<ext>.")
							})
						})
					})

					when("launch bom", func() {
						when("there is a bom in launch.toml", func() {
							it("warns and includes the bom", func() {
								h.Mkfile(t,
									"[[bom]]\n"+
										`name = "some-dep"`+"\n"+
										"[bom.metadata]\n"+
										`version = "some-version"`+"\n",
									filepath.Join(appDir, "launch-A-v1.toml"),
								)

								br, err := executor.Build(descriptor, inputs, logger)
								h.AssertNil(t, err)

								h.AssertEq(t, br.LaunchBOM, []buildpack.BOMEntry{
									{
										Require: buildpack.Require{
											Name:     "some-dep",
											Metadata: map[string]interface{}{"version": "some-version"},
										},
										Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"}, // no api, no homepage
									},
								})
								assertLogEntry(t, logHandler, "BOM table is deprecated in this buildpack api version, though it remains supported for backwards compatibility. Buildpack authors should write BOM information to <layer>.sbom.<ext>, launch.sbom.<ext>, or build.sbom.<ext>.")
							})
						})

						when("there is a bom in launch.toml and SBOM files", func() {
							it("does not warn and does not include the bom", func() {
								h.Mkfile(t,
									"[[bom]]\n"+
										`name = "some-dep"`+"\n"+
										"[bom.metadata]\n"+
										`version = "some-version"`+"\n",
									filepath.Join(appDir, "launch-A-v1.toml"),
								)

								buildpackID := descriptor.Buildpack.ID
								descriptor.Buildpack.SBOM = []string{"application/vnd.cyclonedx+json"}

								h.Mkdir(t,
									filepath.Join(layersDir, buildpackID))
								h.Mkfile(t, `{"key": "some-bom-content"}`,
									filepath.Join(layersDir, buildpackID, "launch.sbom.cdx.json"))

								br, err := executor.Build(descriptor, inputs, logger)
								h.AssertNil(t, err)

								h.AssertEq(t, br.LaunchBOM, []buildpack.BOMEntry{
									{
										Require: buildpack.Require{
											Name:     "some-dep",
											Metadata: map[string]interface{}{"version": "some-version"},
										},
										Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"}, // no api, no homepage
									},
								})
								h.AssertEq(t, br.BOMFiles, []buildpack.BOMFile{
									{
										BuildpackID: buildpackID,
										LayerName:   "",
										LayerType:   buildpack.LayerTypeLaunch,
										Path:        filepath.Join(layersDir, buildpackID, "launch.sbom.cdx.json"),
									},
								})
								assertLogEntryNotContains(t, logHandler, "BOM table is deprecated in this buildpack api version, though it remains supported for backwards compatibility. Buildpack authors should write BOM information to <layer>.sbom.<ext>, launch.sbom.<ext>, or build.sbom.<ext>.")
							})
						})

						it("errors when there is a bom in launch.toml with a top-level version", func() {
							h.Mkfile(t,
								"[[bom]]\n"+
									`name = "some-dep"`+"\n"+
									`version = "some-version"`+"\n",
								filepath.Join(appDir, "launch-A-v1.toml"),
							)

							_, err := executor.Build(descriptor, inputs, logger)
							h.AssertError(t, err, "bom entry 'some-dep' has a top level version which is not allowed. The buildpack should instead set metadata.version")
						})
					})

					when("SBOM files", func() {
						it("includes any SBOM files", func() {
							buildpackID := descriptor.Buildpack.ID
							descriptor.Buildpack.SBOM = []string{"application/vnd.cyclonedx+json;version=1.3"}
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

							br, err := executor.Build(descriptor, inputs, logger)
							h.AssertNil(t, err)

							h.AssertEq(t, buildpack.BuildOutputs{
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

						it("errors if there are unsupported extensions", func() {
							buildpackID := descriptor.Buildpack.ID
							descriptor.Buildpack.SBOM = []string{"application/vnd.cyclonedx+json", "application/spdx+json", "application/vnd.syft+json"}
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

							_, err := executor.Build(descriptor, inputs, logger)
							h.AssertError(t, err, fmt.Sprintf("unsupported SBOM file format: '%s'", filepath.Join(layersDir, buildpackID, fmt.Sprintf("%s.sbom.some-unknown-format.json", layerName))))
						})

						it("errors if there are undeclared media types", func() {
							buildpackID := descriptor.Buildpack.ID
							descriptor.Buildpack.SBOM = []string{"application/vnd.cyclonedx+json"}

							h.Mkdir(t,
								filepath.Join(layersDir, buildpackID))
							h.Mkfile(t, `{"key": "some-bom-content"}`,
								filepath.Join(layersDir, buildpackID, "launch.sbom.spdx.json"))

							_, err := executor.Build(descriptor, inputs, logger)
							h.AssertError(t, err, fmt.Sprintf("validating SBOM file '%s' for buildpack: 'A@v1': undeclared SBOM media type: 'application/spdx+json'", filepath.Join(layersDir, buildpackID, "launch.sbom.spdx.json")))
						})
					})

					when("labels", func() {
						it("includes labels", func() {
							h.Mkfile(t,
								"[[labels]]\n"+
									`key = "some-key"`+"\n"+
									`value = "some-value"`+"\n"+
									"[[labels]]\n"+
									`key = "some-other-key"`+"\n"+
									`value = "some-other-value"`+"\n",
								filepath.Join(appDir, "launch-A-v1.toml"),
							)

							br, err := executor.Build(descriptor, inputs, logger)
							h.AssertNil(t, err)

							h.AssertEq(t, br.Labels, []buildpack.Label{
								{Key: "some-key", Value: "some-value"},
								{Key: "some-other-key", Value: "some-other-value"},
							})
						})
					})

					when("met requires", func() {
						it("are derived from build.toml", func() {
							inputs.Plan = buildpack.Plan{
								Entries: []buildpack.Require{
									{Name: "some-dep"},
									{Name: "some-other-dep"},
									{Name: "some-unmet-dep"},
								},
							}
							h.Mkfile(t,
								"[[unmet]]\n"+
									`name = "some-unmet-dep"`+"\n",
								filepath.Join(appDir, "build-A-v1.toml"),
							)

							br, err := executor.Build(descriptor, inputs, logger)
							h.AssertNil(t, err)

							h.AssertEq(t, br.MetRequires, []string{"some-dep", "some-other-dep"})
						})

						when("there are invalid unmet entries", func() {
							it("errors when name is missing", func() {
								h.Mkfile(t,
									"[[unmet]]\n",
									filepath.Join(appDir, "build-A-v1.toml"),
								)
								_, err := executor.Build(descriptor, inputs, logger)
								h.AssertNotNil(t, err)
								expected := "name is required"
								h.AssertStringContains(t, err.Error(), expected)
							})

							it("errors when name is invalid", func() {
								h.Mkfile(t,
									"[[unmet]]\n"+
										`name = "unknown-dep"`+"\n",
									filepath.Join(appDir, "build-A-v1.toml"),
								)
								_, err := executor.Build(descriptor, inputs, logger)
								h.AssertNotNil(t, err)
								expected := "must match a requested dependency"
								h.AssertStringContains(t, err.Error(), expected)
							})
						})
					})

					when("processes", func() {
						it("includes processes and uses the default value that is set", func() {
							h.Mkfile(t,
								`[[processes]]`+"\n"+
									`type = "some-type"`+"\n"+
									`command = ["some-cmd"]`+"\n"+
									`default = true`+"\n"+
									`[[processes]]`+"\n"+
									`type = "web"`+"\n"+
									`command = ["other-cmd"]`+"\n",
								// default is false and therefore doesn't appear
								filepath.Join(appDir, "launch-A-v1.toml"),
							)
							br, err := executor.Build(descriptor, inputs, logger)
							h.AssertNil(t, err)

							h.AssertEq(t, br.Processes, []launch.Process{
								{Type: "some-type", Command: launch.NewRawCommand([]string{"some-cmd"}), BuildpackID: "A", Default: true, Direct: true},
								{Type: "web", Command: launch.NewRawCommand([]string{"other-cmd"}), BuildpackID: "A", Default: false, Direct: true},
							}, processCmpOpts...)
						})

						when("there is more than one default=true process", func() {
							it("errors when the processes have the same type", func() {
								h.Mkfile(t,
									`[[processes]]`+"\n"+
										`type = "some-type"`+"\n"+
										`command = ["some-cmd"]`+"\n"+
										`default = true`+"\n"+
										`[[processes]]`+"\n"+
										`type = "some-type"`+"\n"+
										`command = ["some-other-cmd"]`+"\n"+
										`default = true`+"\n",
									filepath.Join(appDir, "launch-A-v1.toml"),
								)
								_, err := executor.Build(descriptor, inputs, logger)
								h.AssertNotNil(t, err)
								expected := "multiple default process types aren't allowed"
								h.AssertStringContains(t, err.Error(), expected)
							})

							it("errors when the processes have different types", func() {
								h.Mkfile(t,
									`[[processes]]`+"\n"+
										`type = "some-type"`+"\n"+
										`command = ["some-cmd"]`+"\n"+
										`default = true`+"\n"+
										`[[processes]]`+"\n"+
										`type = "other-type"`+"\n"+
										`command = ["other-cmd"]`+"\n"+
										`default = true`+"\n",
									filepath.Join(appDir, "launch-A-v1.toml"),
								)
								_, err := executor.Build(descriptor, inputs, logger)
								h.AssertNotNil(t, err)
								expected := "multiple default process types aren't allowed"
								h.AssertStringContains(t, err.Error(), expected)
							})
						})

						it("does not allow string command", func() {
							h.Mkfile(t,
								"[[processes]]\n"+
									`command = "some-cmd"`,
								filepath.Join(appDir, "launch-A-v1.toml"),
							)
							_, err := executor.Build(descriptor, inputs, logger)
							h.AssertError(t, err, "toml: line 2 (last key \"processes.command\"): incompatible types: TOML value has type string; destination has type slice")
						})

						it("preserves command args", func() {
							h.Mkfile(t,
								"[[processes]]\n"+
									`command = ["some-cmd", "cmd-arg"]`+"\n"+
									`args = ["first-arg"]`,
								filepath.Join(appDir, "launch-A-v1.toml"),
							)
							br, err := executor.Build(descriptor, inputs, logger)
							h.AssertNil(t, err)
							h.AssertEq(t, len(br.Processes), 1)
							h.AssertEq(t, br.Processes[0].Command.Entries, []string{"some-cmd", "cmd-arg"})
							h.AssertEq(t, br.Processes[0].Args[0], "first-arg")
						})

						it("returns direct=true for processes", func() {
							h.Mkfile(t,
								"[[processes]]\n"+
									`command = ["some-cmd"]`+"\n"+
									`args = ["first-arg"]`,
								filepath.Join(appDir, "launch-A-v1.toml"),
							)
							br, err := executor.Build(descriptor, inputs, logger)
							h.AssertNil(t, err)
							h.AssertEq(t, len(br.Processes), 1)
							h.AssertEq(t, br.Processes[0].Command.Entries, []string{"some-cmd"})
							h.AssertEq(t, br.Processes[0].Direct, true)
						})

						it("does not allow direct flag", func() {
							h.Mkfile(t,
								"[[processes]]\n"+
									`command = ["some-cmd"]`+"\n"+
									`direct = false`,
								filepath.Join(appDir, "launch-A-v1.toml"),
							)
							_, err := executor.Build(descriptor, inputs, logger)
							h.AssertError(t, err, "process.direct is not supported on this buildpack version")
						})

						it("sets the working directory", func() {
							h.Mkfile(t,
								"[[processes]]\n"+
									`command = ["some-cmd"]`+"\n"+
									`working-dir = "/working-directory"`,
								filepath.Join(appDir, "launch-A-v1.toml"),
							)
							br, err := executor.Build(descriptor, inputs, logger)
							h.AssertNil(t, err)
							h.AssertEq(t, len(br.Processes), 1)
							h.AssertEq(t, br.Processes[0].WorkingDirectory, "/working-directory")
						})
					})

					when("slices", func() {
						it("includes slices", func() {
							h.Mkfile(t,
								"[[slices]]\n"+
									`paths = ["some-path", "some-other-path"]`+"\n",
								filepath.Join(appDir, "launch-A-v1.toml"),
							)

							br, err := executor.Build(descriptor, inputs, logger)
							h.AssertNil(t, err)

							h.AssertEq(t, br.Slices, []layers.Slice{{Paths: []string{"some-path", "some-other-path"}}})
						})
					})
				})

				when("buildpack api < 0.8", func() {
					it.Before(func() {
						descriptor.WithAPI = "0.7"
					})

					it("does not set environment variables for positional arguments", func() {
						_, err := executor.Build(descriptor, inputs, logger)

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

					when("launch.toml", func() {
						it("ignores process working directory and warns", func() {
							h.Mkfile(t,
								"[[processes]]\n"+
									`command = "echo"`+"\n"+
									`working-dir = "/working-directory"`+"\n"+
									`type = "some-type"`+"\n",
								filepath.Join(appDir, "launch-A-v1.toml"),
							)
							br, err := executor.Build(descriptor, inputs, logger)
							h.AssertNil(t, err)
							h.AssertEq(t, len(br.Processes), 1)
							h.AssertEq(t, br.Processes[0].WorkingDirectory, "")
							assertLogEntry(t, logHandler, "Warning: process working directory isn't supported in this buildpack api version. Ignoring working directory for process 'some-type'")
						})
					})
				})

				when("buildpack api < 0.9", func() {
					it.Before(func() {
						descriptor.WithAPI = "0.8"
					})

					it("allows setting direct", func() {
						h.Mkfile(t,
							"[[processes]]\n"+
								`command = "some-cmd"`+"\n"+
								`direct = false`,
							filepath.Join(appDir, "launch-A-v1.toml"),
						)
						br, err := executor.Build(descriptor, inputs, logger)
						h.AssertNil(t, err)
						h.AssertEq(t, len(br.Processes), 1)
						h.AssertEq(t, br.Processes[0].Direct, false)
					})

					it("allows setting a single command string", func() {
						h.Mkfile(t,
							"[[processes]]\n"+
								`command = "some-command"`,
							filepath.Join(appDir, "launch-A-v1.toml"),
						)
						br, err := executor.Build(descriptor, inputs, logger)
						h.AssertNil(t, err)
						h.AssertEq(t, len(br.Processes), 1)
						h.AssertEq(t, br.Processes[0].Command.Entries, []string{"some-command"})
					})

					it("does not allow commands as list of string", func() {
						h.Mkfile(t,
							"[[processes]]\n"+
								`command = ["some-cmd"]`+"\n"+
								`direct = false`,
							filepath.Join(appDir, "launch-A-v1.toml"),
						)
						_, err := executor.Build(descriptor, inputs, logger)
						h.AssertError(t, err, "toml: line 2 (last key \"processes.command\"): incompatible types: TOML value has type []any; destination has type string")
					})
				})
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
