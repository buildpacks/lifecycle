package lifecycle_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

func TestBuilder(t *testing.T) {
	spec.Run(t, "Builder", testBuilder, spec.Report(report.Terminal{}))
}

//go:generate mockgen -package testmock -destination testmock/env.go github.com/buildpacks/lifecycle BuildEnv
//go:generate mockgen -package testmock -destination testmock/buildpack_store.go github.com/buildpacks/lifecycle BuildpackStore
//go:generate mockgen -package testmock -destination testmock/buildpack.go github.com/buildpacks/lifecycle Buildpack

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		builder        *lifecycle.Builder
		mockCtrl       *gomock.Controller
		buildpackStore *testmock.MockBuildpackStore
		stdout, stderr *bytes.Buffer
		tmpDir         string
		platformDir    string
		appDir         string
		layersDir      string
		config         buildpack.BuildConfig
		logHandler     = memory.New()
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		buildpackStore = testmock.NewMockBuildpackStore(mockCtrl)

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

		builder = &lifecycle.Builder{
			AppDir:      appDir,
			LayersDir:   layersDir,
			PlatformDir: platformDir,
			Platform:    platform.NewPlatform(api.Platform.Latest().String()),
			Group: buildpack.Group{
				Group: []buildpack.GroupBuildpack{
					{ID: "A", Version: "v1", API: api.Buildpack.Latest().String(), Homepage: "Buildpack A Homepage"},
					{ID: "B", Version: "v2", API: api.Buildpack.Latest().String()},
				},
			},
			Out:            stdout,
			Err:            stderr,
			Logger:         &log.Logger{Handler: logHandler},
			BuildpackStore: buildpackStore,
		}

		config, err = builder.BuildConfig()
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
		mockCtrl.Finish()
	})

	when("#Build", func() {
		when("building succeeds", func() {
			it("should provide a subset of the build plan to each buildpack", func() {
				builder.Plan = platform.BuildPlan{
					Entries: []platform.BuildPlanEntry{
						{
							Providers: []buildpack.GroupBuildpack{
								{ID: "A", Version: "v1"},
								{ID: "B", Version: "v2"},
							},
							Requires: []buildpack.Require{
								{Name: "some-dep", Version: "v1"}, // not provided to buildpack B because it is met
							},
						},
						{
							Providers: []buildpack.GroupBuildpack{
								{ID: "A", Version: "v1"},
								{ID: "B", Version: "v2"},
							},
							Requires: []buildpack.Require{
								{Name: "some-unmet-dep", Version: "v2"}, // provided to buildpack B because it is unmet
							},
						},
						{
							Providers: []buildpack.GroupBuildpack{
								{ID: "B", Version: "v2"},
							},
							Requires: []buildpack.Require{
								{Name: "other-dep", Version: "v4"}, // only provided to buildpack B
							},
						},
					},
				}
				bpA := testmock.NewMockBuildpack(mockCtrl)
				bpB := testmock.NewMockBuildpack(mockCtrl)
				buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
				expectedPlanA := buildpack.Plan{Entries: []buildpack.Require{
					{Name: "some-dep", Version: "v1"},
					{Name: "some-unmet-dep", Version: "v2"},
				}}
				bpA.EXPECT().Build(expectedPlanA, config, gomock.Any()).Return(buildpack.BuildResult{
					MetRequires: []string{"some-dep"},
				}, nil)
				buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
				expectedPlanB := buildpack.Plan{Entries: []buildpack.Require{
					{Name: "some-unmet-dep", Version: "v2"},
					{Name: "other-dep", Version: "v4"},
				}}
				bpB.EXPECT().Build(expectedPlanB, config, gomock.Any())

				_, err := builder.Build()
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
			})

			it("should provide the correct environment to each buildpack", func() {
				bpA := &fakeBp{}
				bpB := testmock.NewMockBuildpack(mockCtrl)
				expectedEnv := env.NewBuildEnv(append(os.Environ(), "HOME=modified-by-bpA"))

				buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
				buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
				bpB.EXPECT().Build(gomock.Any(), config, expectedEnv)

				_, err := builder.Build()
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
			})

			it("copies any created BOM files to the correct locations", func() {
				bpA := testmock.NewMockBuildpack(mockCtrl)
				bpB := testmock.NewMockBuildpack(mockCtrl)
				buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
				buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)

				bomFilePath1 := filepath.Join(layersDir, "launch.sbom.cdx.json")
				bomFilePath2 := filepath.Join(layersDir, "build.sbom.cdx.json")
				bomFilePath3 := filepath.Join(layersDir, "layer-b1.sbom.cdx.json")
				bomFilePath4 := filepath.Join(layersDir, "layer-b2.sbom.cdx.json")
				h.Mkfile(t, `{"key": "some-bom-content-1"}`, bomFilePath1)
				h.Mkfile(t, `{"key": "some-bom-content-2"}`, bomFilePath2)
				h.Mkfile(t, `{"key": "some-bom-content-3"}`, bomFilePath3)
				h.Mkfile(t, `{"key": "some-bom-content-4"}`, bomFilePath4)

				bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
					BOMFiles: []buildpack.BOMFile{
						{
							BuildpackID: "A",
							LayerName:   "",
							LayerType:   buildpack.LayerTypeLaunch,
							Path:        bomFilePath1,
						},
						{
							BuildpackID: "A",
							LayerName:   "",
							LayerType:   buildpack.LayerTypeBuild,
							Path:        bomFilePath2,
						},
					},
				}, nil)
				bpB.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
					BOMFiles: []buildpack.BOMFile{
						{
							BuildpackID: "B",
							LayerName:   "layer-b1",
							LayerType:   buildpack.LayerTypeBuild,
							Path:        bomFilePath3,
						},
						{
							BuildpackID: "B",
							LayerName:   "layer-b1",
							LayerType:   buildpack.LayerTypeCache,
							Path:        bomFilePath3,
						},
						{
							BuildpackID: "B",
							LayerName:   "layer-b2",
							LayerType:   buildpack.LayerTypeLaunch,
							Path:        bomFilePath4,
						},
					},
				}, nil)

				_, err := builder.Build()
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				result := h.MustReadFile(t, filepath.Join(layersDir, "sbom", "launch", "A", "sbom.cdx.json"))
				h.AssertEq(t, string(result), `{"key": "some-bom-content-1"}`)

				result = h.MustReadFile(t, filepath.Join(layersDir, "sbom", "build", "A", "sbom.cdx.json"))
				h.AssertEq(t, string(result), `{"key": "some-bom-content-2"}`)

				result = h.MustReadFile(t, filepath.Join(layersDir, "sbom", "build", "B", "layer-b1", "sbom.cdx.json"))
				h.AssertEq(t, string(result), `{"key": "some-bom-content-3"}`)

				result = h.MustReadFile(t, filepath.Join(layersDir, "sbom", "cache", "B", "layer-b1", "sbom.cdx.json"))
				h.AssertEq(t, string(result), `{"key": "some-bom-content-3"}`)

				result = h.MustReadFile(t, filepath.Join(layersDir, "sbom", "launch", "B", "layer-b2", "sbom.cdx.json"))
				h.AssertEq(t, string(result), `{"key": "some-bom-content-4"}`)
			})

			it("returns an error for any unsupported BOM formats", func() {
				bpA := testmock.NewMockBuildpack(mockCtrl)
				bpB := testmock.NewMockBuildpack(mockCtrl)
				buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
				buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)

				bomFilePath1 := filepath.Join(layersDir, "launch.sbom.cdx.json")
				bomFilePath2 := filepath.Join(layersDir, "layer-b.sbom.some-unknown-format.json")
				h.Mkfile(t, `{"key": "some-bom-content-a"}`, bomFilePath1)
				h.Mkfile(t, `{"key": "some-bom-content-b"}`, bomFilePath2)

				bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
					BOMFiles: []buildpack.BOMFile{
						{
							BuildpackID: "A",
							LayerName:   "",
							LayerType:   buildpack.LayerTypeLaunch,
							Path:        bomFilePath1,
						},
					},
				}, nil)
				bpB.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
					BOMFiles: []buildpack.BOMFile{
						{
							BuildpackID: "B",
							LayerName:   "layer-b",
							LayerType:   buildpack.LayerTypeBuild,
							Path:        bomFilePath2,
						},
					},
				}, nil)

				_, err := builder.Build()
				h.AssertError(t, err, fmt.Sprintf("unsupported sbom format: '%s'", bomFilePath2))
			})

			it("cleans the /layers/sbom directory before building", func() {
				oldDir := filepath.Join(layersDir, "sbom", "launch", "undetected-buildpack")
				h.Mkdir(t, oldDir)
				oldFile := filepath.Join(oldDir, "launch.sbom.cdx.json")
				h.Mkfile(t, `{"key": "some-bom-content"}`, oldFile)
				bpA := testmock.NewMockBuildpack(mockCtrl)
				bpB := testmock.NewMockBuildpack(mockCtrl)
				buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
				buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
				bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{}, nil)
				bpB.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{}, nil)

				_, err := builder.Build()
				h.AssertNil(t, err)

				h.AssertPathDoesNotExist(t, oldFile)
			})

			when("build metadata", func() {
				when("bom", func() {
					it("should aggregate BOM from each buildpack", func() {
						builder.Group.Group = []buildpack.GroupBuildpack{
							{ID: "A", Version: "v1", API: "0.5", Homepage: "Buildpack A Homepage"},
							{ID: "B", Version: "v2", API: "0.2"},
						}

						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
							BOM: []buildpack.BOMEntry{
								{
									Require: buildpack.Require{
										Name:     "dep1",
										Metadata: map[string]interface{}{"version": "v1"},
									},
									Buildpack: buildpack.GroupBuildpack{ID: "A", Version: "v1"},
								},
							},
						}, nil)
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
							BOM: []buildpack.BOMEntry{
								{
									Require: buildpack.Require{
										Name:     "dep2",
										Metadata: map[string]interface{}{"version": "v1"},
									},
									Buildpack: buildpack.GroupBuildpack{ID: "B", Version: "v2"},
								},
							},
						}, nil)

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}
						if s := cmp.Diff(metadata.BOM, []buildpack.BOMEntry{
							{
								Require: buildpack.Require{
									Name:     "dep1",
									Version:  "",
									Metadata: map[string]interface{}{"version": string("v1")},
								},
								Buildpack: buildpack.GroupBuildpack{ID: "A", Version: "v1"},
							},
							{
								Require: buildpack.Require{
									Name:     "dep2",
									Version:  "",
									Metadata: map[string]interface{}{"version": string("v1")},
								},
								Buildpack: buildpack.GroupBuildpack{ID: "B", Version: "v2"},
							},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})
				})

				when("buildpacks", func() {
					it("should include builder buildpacks", func() {
						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config, gomock.Any())
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config, gomock.Any())

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}
						if s := cmp.Diff(metadata.Buildpacks, []buildpack.GroupBuildpack{
							{ID: "A", Version: "v1", API: api.Buildpack.Latest().String(), Homepage: "Buildpack A Homepage"},
							{ID: "B", Version: "v2", API: api.Buildpack.Latest().String()},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})
				})

				when("labels", func() {
					it("should aggregate labels from each buildpack", func() {
						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
							Labels: []buildpack.Label{
								{Key: "some-bpA-key", Value: "some-bpA-value"},
								{Key: "some-other-bpA-key", Value: "some-other-bpA-value"},
							},
						}, nil)
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
							Labels: []buildpack.Label{
								{Key: "some-bpB-key", Value: "some-bpB-value"},
								{Key: "some-other-bpB-key", Value: "some-other-bpB-value"},
							},
						}, nil)

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}
						if s := cmp.Diff(metadata.Labels, []buildpack.Label{
							{Key: "some-bpA-key", Value: "some-bpA-value"},
							{Key: "some-other-bpA-key", Value: "some-other-bpA-value"},
							{Key: "some-bpB-key", Value: "some-bpB-value"},
							{Key: "some-other-bpB-key", Value: "some-other-bpB-value"},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})
				})

				when("processes", func() {
					it("should override identical processes from earlier buildpacks", func() {
						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
							Processes: []launch.Process{
								{
									Type:        "some-type",
									Command:     "some-command",
									Args:        []string{"some-arg"},
									Direct:      true,
									BuildpackID: "A",
								},
								{
									Type:        "override-type",
									Command:     "bpA-command",
									Args:        []string{"bpA-arg"},
									Direct:      true,
									BuildpackID: "A",
								},
							},
						}, nil)
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
							Processes: []launch.Process{
								{
									Type:        "some-other-type",
									Command:     "some-other-command",
									Args:        []string{"some-other-arg"},
									Direct:      true,
									BuildpackID: "B",
								},
								{
									Type:        "override-type",
									Command:     "bpB-command",
									Args:        []string{"bpB-arg"},
									Direct:      false,
									BuildpackID: "B",
								},
							},
						}, nil)

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}
						if s := cmp.Diff(metadata.Processes, []launch.Process{
							{
								Type:        "override-type",
								Command:     "bpB-command",
								Args:        []string{"bpB-arg"},
								Direct:      false,
								BuildpackID: "B",
							},
							{
								Type:        "some-other-type",
								Command:     "some-other-command",
								Args:        []string{"some-other-arg"},
								Direct:      true,
								BuildpackID: "B",
							},
							{
								Type:        "some-type",
								Command:     "some-command",
								Args:        []string{"some-arg"},
								Direct:      true,
								BuildpackID: "A",
							},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
						h.AssertEq(t, metadata.BuildpackDefaultProcessType, "")
					})

					when("multiple default process types", func() {
						it.Before(func() {
							builder.Group.Group = []buildpack.GroupBuildpack{
								{ID: "A", Version: "v1", API: api.Buildpack.Latest().String()},
								{ID: "B", Version: "v2", API: api.Buildpack.Latest().String()},
								{ID: "C", Version: "v3", API: api.Buildpack.Latest().String()},
							}
						})

						it("last default process type wins", func() {
							bpA := testmock.NewMockBuildpack(mockCtrl)
							buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
							bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
								Processes: []launch.Process{
									{
										Type:        "override-type",
										Command:     "bpA-command",
										Args:        []string{"bpA-arg"},
										Direct:      true,
										BuildpackID: "A",
										Default:     true,
									},
								},
							}, nil)
							bpB := testmock.NewMockBuildpack(mockCtrl)
							buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
							bpB.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
								Processes: []launch.Process{
									{
										Type:        "some-type",
										Command:     "bpB-command",
										Args:        []string{"bpB-arg"},
										Direct:      false,
										BuildpackID: "B",
										Default:     true,
									},
								},
							}, nil)

							bpC := testmock.NewMockBuildpack(mockCtrl)
							buildpackStore.EXPECT().Lookup("C", "v3").Return(bpC, nil)
							bpC.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
								Processes: []launch.Process{
									{
										Type:        "override-type",
										Command:     "bpC-command",
										Args:        []string{"bpC-arg"},
										Direct:      false,
										BuildpackID: "C",
									},
								},
							}, nil)

							metadata, err := builder.Build()
							if err != nil {
								t.Fatalf("Unexpected error:\n%s\n", err)
							}

							if s := cmp.Diff(metadata.Processes, []launch.Process{
								{
									Type:        "override-type",
									Command:     "bpC-command",
									Args:        []string{"bpC-arg"},
									Direct:      false,
									BuildpackID: "C",
								},
								{
									Type:        "some-type",
									Command:     "bpB-command",
									Args:        []string{"bpB-arg"},
									Direct:      false,
									BuildpackID: "B",
								},
							}); s != "" {
								t.Fatalf("Unexpected:\n%s\n", s)
							}
							h.AssertEq(t, metadata.BuildpackDefaultProcessType, "some-type")
						})
					})

					when("overriding default process type, with a non-default process type", func() {
						it.Before(func() {
							builder.Group.Group = []buildpack.GroupBuildpack{
								{ID: "A", Version: "v1", API: api.Buildpack.Latest().String()},
								{ID: "B", Version: "v2", API: api.Buildpack.Latest().String()},
								{ID: "C", Version: "v3", API: api.Buildpack.Latest().String()},
							}
						})

						it("should warn and not set any default process", func() {
							bpB := testmock.NewMockBuildpack(mockCtrl)
							buildpackStore.EXPECT().Lookup("A", "v1").Return(bpB, nil)
							bpB.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
								Processes: []launch.Process{
									{
										Type:        "some-type",
										Command:     "bpA-command",
										Args:        []string{"bpA-arg"},
										Direct:      false,
										BuildpackID: "A",
										Default:     true,
									},
								},
							}, nil)

							bpA := testmock.NewMockBuildpack(mockCtrl)
							buildpackStore.EXPECT().Lookup("B", "v2").Return(bpA, nil)
							bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
								Processes: []launch.Process{
									{
										Type:        "override-type",
										Command:     "bpB-command",
										Args:        []string{"bpB-arg"},
										Direct:      true,
										BuildpackID: "B",
										Default:     true,
									},
								},
							}, nil)

							bpC := testmock.NewMockBuildpack(mockCtrl)
							buildpackStore.EXPECT().Lookup("C", "v3").Return(bpC, nil)
							bpC.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
								Processes: []launch.Process{
									{
										Type:        "override-type",
										Command:     "bpC-command",
										Args:        []string{"bpC-arg"},
										Direct:      false,
										BuildpackID: "C",
									},
								},
							}, nil)

							metadata, err := builder.Build()
							if err != nil {
								t.Fatalf("Unexpected error:\n%s\n", err)
							}
							if s := cmp.Diff(metadata.Processes, []launch.Process{
								{
									Type:        "override-type",
									Command:     "bpC-command",
									Args:        []string{"bpC-arg"},
									Direct:      false,
									BuildpackID: "C",
								},
								{
									Type:        "some-type",
									Command:     "bpA-command",
									Args:        []string{"bpA-arg"},
									Direct:      false,
									BuildpackID: "A",
								},
							}); s != "" {
								t.Fatalf("Unexpected:\n%s\n", s)
							}

							expected := "Warning: redefining the following default process type with a process not marked as default: override-type"
							assertLogEntry(t, logHandler, expected)

							h.AssertEq(t, metadata.BuildpackDefaultProcessType, "")
						})
					})

					when("there is a web process", func() {
						when("buildpack API >= 0.6", func() {
							it.Before(func() {
								builder.Group.Group = []buildpack.GroupBuildpack{
									{ID: "A", Version: "v1", API: api.Buildpack.Latest().String()},
								}
							})
							it("shouldn't set it as a default process", func() {
								bpA := testmock.NewMockBuildpack(mockCtrl)
								buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
								bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
									Processes: []launch.Process{
										{
											Type:        "web",
											Command:     "web-cmd",
											Args:        []string{"web-arg"},
											Direct:      false,
											BuildpackID: "A",
											Default:     false,
										},
									},
								}, nil)

								metadata, err := builder.Build()
								if err != nil {
									t.Fatalf("Unexpected error:\n%s\n", err)
								}

								if s := cmp.Diff(metadata.Processes, []launch.Process{
									{
										Type:        "web",
										Command:     "web-cmd",
										Args:        []string{"web-arg"},
										Direct:      false,
										BuildpackID: "A",
										Default:     false,
									},
								}); s != "" {
									t.Fatalf("Unexpected:\n%s\n", s)
								}
								h.AssertEq(t, metadata.BuildpackDefaultProcessType, "")
							})
						})

						when("buildpack api < 0.6", func() {
							it.Before(func() {
								builder.Group.Group = []buildpack.GroupBuildpack{
									{ID: "A", Version: "v1", API: "0.5"},
								}
							})
							it("should set it as a default process", func() {
								bpA := testmock.NewMockBuildpack(mockCtrl)
								buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
								bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
									Processes: []launch.Process{
										{
											Type:        "web",
											Command:     "web-cmd",
											Args:        []string{"web-arg"},
											Direct:      false,
											BuildpackID: "A",
											Default:     false,
										},
										{
											Type:        "not-web",
											Command:     "not-web-cmd",
											Args:        []string{"not-web-arg"},
											Direct:      true,
											BuildpackID: "A",
											Default:     false,
										},
									},
								}, nil)

								metadata, err := builder.Build()
								if err != nil {
									t.Fatalf("Unexpected error:\n%s\n", err)
								}

								if s := cmp.Diff(metadata.Processes, []launch.Process{
									{
										Type:        "not-web",
										Command:     "not-web-cmd",
										Args:        []string{"not-web-arg"},
										Direct:      true,
										BuildpackID: "A",
									},
									{
										Type:        "web",
										Command:     "web-cmd",
										Args:        []string{"web-arg"},
										Direct:      false,
										BuildpackID: "A",
									},
								}); s != "" {
									t.Fatalf("Unexpected:\n%s\n", s)
								}
								h.AssertEq(t, metadata.BuildpackDefaultProcessType, "web")
							})
						})
					})
				})

				when("slices", func() {
					it("should aggregate slices from each buildpack", func() {
						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
							Slices: []layers.Slice{
								{Paths: []string{"some-bpA-path", "some-other-bpA-path"}},
								{Paths: []string{"duplicate-path"}},
								{Paths: []string{"extra-path"}},
							},
						}, nil)
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
							Slices: []layers.Slice{
								{Paths: []string{"some-bpB-path", "some-other-bpB-path"}},
								{Paths: []string{"duplicate-path"}},
							},
						}, nil)

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}
						if s := cmp.Diff(metadata.Slices, []layers.Slice{
							{Paths: []string{"some-bpA-path", "some-other-bpA-path"}},
							{Paths: []string{"duplicate-path"}},
							{Paths: []string{"extra-path"}},
							{Paths: []string{"some-bpB-path", "some-other-bpB-path"}},
							{Paths: []string{"duplicate-path"}},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})
				})
			})
		})

		when("building fails", func() {
			when("first buildpack build fails", func() {
				it("should error", func() {
					bpA := testmock.NewMockBuildpack(mockCtrl)
					buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
					bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{}, errors.New("some error"))

					if _, err := builder.Build(); err == nil {
						t.Fatal("Expected error.\n")
					} else if !strings.Contains(err.Error(), "some error") {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})
			})

			when("later buildpack build fails", func() {
				it("should error", func() {
					bpA := testmock.NewMockBuildpack(mockCtrl)
					buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
					bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{}, nil)
					bpB := testmock.NewMockBuildpack(mockCtrl)
					buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
					bpB.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{}, errors.New("some error"))

					if _, err := builder.Build(); err == nil {
						t.Fatal("Expected error.\n")
					} else if !strings.Contains(err.Error(), "some error") {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})
			})
		})

		when("platform api < 0.4", func() {
			it.Before(func() {
				builder.Platform = platform.NewPlatform("0.3")
			})

			when("build metadata", func() {
				when("bom", func() {
					it("should convert metadata.version to top level version", func() {
						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
							BOM: []buildpack.BOMEntry{
								{
									Require: buildpack.Require{
										Name:     "dep1",
										Metadata: map[string]interface{}{"version": string("v1")},
									},
									Buildpack: buildpack.GroupBuildpack{ID: "A", Version: "v1"},
								},
							},
						}, nil)
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config, gomock.Any())

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}

						if s := cmp.Diff(metadata.BOM, []buildpack.BOMEntry{
							{
								Require: buildpack.Require{
									Name:     "dep1",
									Version:  "v1",
									Metadata: map[string]interface{}{"version": string("v1")},
								},
								Buildpack: buildpack.GroupBuildpack{ID: "A", Version: "v1"},
							},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})
				})
			})
		})

		when("platform api < 0.6", func() {
			it.Before(func() {
				builder.Platform = platform.NewPlatform("0.5")
			})

			when("there is a web process", func() {
				when("buildpack API >= 0.6", func() {
					it.Before(func() {
						builder.Group.Group = []buildpack.GroupBuildpack{
							{ID: "A", Version: "v1", API: api.Buildpack.Latest().String()},
						}
					})

					it("shouldn't set it as a default process", func() {
						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
							Processes: []launch.Process{
								{
									Type:        "web",
									Command:     "web-cmd",
									Args:        []string{"web-arg"},
									Direct:      false,
									BuildpackID: "A",
									Default:     false,
								},
							},
						}, nil)

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}

						if s := cmp.Diff(metadata.Processes, []launch.Process{
							{
								Type:        "web",
								Command:     "web-cmd",
								Args:        []string{"web-arg"},
								Direct:      false,
								BuildpackID: "A",
								Default:     false,
							},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
						h.AssertEq(t, metadata.BuildpackDefaultProcessType, "")
					})
				})

				when("buildpack api < 0.6", func() {
					it.Before(func() {
						builder.Group.Group = []buildpack.GroupBuildpack{
							{ID: "A", Version: "v1", API: "0.5"},
						}
					})

					it("shouldn't set it as a default process", func() {
						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config, gomock.Any()).Return(buildpack.BuildResult{
							Processes: []launch.Process{
								{
									Type:        "web",
									Command:     "web-cmd",
									Args:        []string{"web-arg"},
									Direct:      false,
									BuildpackID: "A",
									Default:     false,
								},
							},
						}, nil)

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}

						if s := cmp.Diff(metadata.Processes, []launch.Process{
							{
								Type:        "web",
								Command:     "web-cmd",
								Args:        []string{"web-arg"},
								Direct:      false,
								BuildpackID: "A",
								Default:     false,
							},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
						h.AssertEq(t, metadata.BuildpackDefaultProcessType, "")
					})
				})
			})
		})
	})
}

type fakeBp struct{}

func (b *fakeBp) Build(bpPlan buildpack.Plan, config buildpack.BuildConfig, bpEnv buildpack.BuildEnv) (buildpack.BuildResult, error) {
	providedEnv, ok := bpEnv.(*env.Env)
	if !ok {
		return buildpack.BuildResult{}, errors.New("failed to cast bpEnv")
	}
	newEnv := env.NewBuildEnv(append(os.Environ(), "HOME=modified-by-bpA"))
	*providedEnv = *newEnv
	return buildpack.BuildResult{}, nil
}

func (b *fakeBp) ConfigFile() *buildpack.Descriptor {
	return nil
}

func (b *fakeBp) Detect(config *buildpack.DetectConfig, bpEnv buildpack.BuildEnv) buildpack.DetectRun {
	return buildpack.DetectRun{}
}
