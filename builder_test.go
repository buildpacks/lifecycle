package lifecycle_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
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
		mockEnv        *testmock.MockBuildEnv
		buildpackStore *testmock.MockBuildpackStore
		stdout, stderr *bytes.Buffer
		tmpDir         string
		platformDir    string
		appDir         string
		layersDir      string
		config         lifecycle.BuildConfig
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		mockEnv = testmock.NewMockBuildEnv(mockCtrl)
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
			PlatformAPI: api.Platform.Latest(),
			Env:         mockEnv,
			Group: lifecycle.BuildpackGroup{
				Group: []lifecycle.GroupBuildpack{
					{ID: "A", Version: "v1", API: latestBuildpackAPI.String(), Homepage: "Buildpack A Homepage"},
					{ID: "B", Version: "v2", API: "0.5"},
				},
			},
			Out:            stdout,
			Err:            stderr,
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
				builder.Plan = lifecycle.BuildPlan{
					Entries: []lifecycle.BuildPlanEntry{
						{
							Providers: []lifecycle.GroupBuildpack{
								{ID: "A", Version: "v1"},
								{ID: "B", Version: "v2"},
							},
							Requires: []lifecycle.Require{
								{Name: "some-dep", Version: "v1"}, // not provided to buildpack B because it is met
							},
						},
						{
							Providers: []lifecycle.GroupBuildpack{
								{ID: "A", Version: "v1"},
								{ID: "B", Version: "v2"},
							},
							Requires: []lifecycle.Require{
								{Name: "some-unmet-dep", Version: "v2"}, // provided to buildpack B because it is unmet
							},
						},
						{
							Providers: []lifecycle.GroupBuildpack{
								{ID: "B", Version: "v2"},
							},
							Requires: []lifecycle.Require{
								{Name: "other-dep", Version: "v4"}, // only provided to buildpack B
							},
						},
					},
				}
				bpA := testmock.NewMockBuildpack(mockCtrl)
				bpB := testmock.NewMockBuildpack(mockCtrl)
				buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
				expectedPlanA := lifecycle.BuildpackPlan{Entries: []lifecycle.Require{
					{Name: "some-dep", Version: "v1"},
					{Name: "some-unmet-dep", Version: "v2"},
				}}
				bpA.EXPECT().Build(expectedPlanA, config).Return(lifecycle.BuildResult{
					MetRequires: []string{"some-dep"},
				}, nil)
				buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
				expectedPlanB := lifecycle.BuildpackPlan{Entries: []lifecycle.Require{
					{Name: "some-unmet-dep", Version: "v2"},
					{Name: "other-dep", Version: "v4"},
				}}
				bpB.EXPECT().Build(expectedPlanB, config)

				_, err := builder.Build()
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
			})

			when("build metadata", func() {
				when("bom", func() {
					it("should aggregate BOM from each buildpack", func() {
						builder.Group.Group = []lifecycle.GroupBuildpack{
							{ID: "A", Version: "v1", API: "0.5", Homepage: "Buildpack A Homepage"},
							{ID: "B", Version: "v2", API: "0.2"},
						}

						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							BOM: []lifecycle.BOMEntry{
								{
									Require: lifecycle.Require{
										Name:     "dep1",
										Metadata: map[string]interface{}{"version": "v1"},
									},
									Buildpack: lifecycle.GroupBuildpack{ID: "A", Version: "v1"},
								},
							},
						}, nil)
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							BOM: []lifecycle.BOMEntry{
								{
									Require: lifecycle.Require{
										Name:     "dep2",
										Metadata: map[string]interface{}{"version": "v1"},
									},
									Buildpack: lifecycle.GroupBuildpack{ID: "B", Version: "v2"},
								},
							},
						}, nil)

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}
						if s := cmp.Diff(metadata.BOM, []lifecycle.BOMEntry{
							{
								Require: lifecycle.Require{
									Name:     "dep1",
									Version:  "",
									Metadata: map[string]interface{}{"version": string("v1")},
								},
								Buildpack: lifecycle.GroupBuildpack{ID: "A", Version: "v1"},
							},
							{
								Require: lifecycle.Require{
									Name:     "dep2",
									Version:  "",
									Metadata: map[string]interface{}{"version": string("v1")},
								},
								Buildpack: lifecycle.GroupBuildpack{ID: "B", Version: "v2"},
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
						bpA.EXPECT().Build(gomock.Any(), config)
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config)

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}
						if s := cmp.Diff(metadata.Buildpacks, []lifecycle.GroupBuildpack{
							{ID: "A", Version: "v1", API: latestBuildpackAPI.String(), Homepage: "Buildpack A Homepage"},
							{ID: "B", Version: "v2", API: "0.5"},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})
				})

				when("labels", func() {
					it("should aggregate labels from each buildpack", func() {
						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							Labels: []lifecycle.Label{
								{Key: "some-bpA-key", Value: "some-bpA-value"},
								{Key: "some-other-bpA-key", Value: "some-other-bpA-value"},
							},
						}, nil)
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							Labels: []lifecycle.Label{
								{Key: "some-bpB-key", Value: "some-bpB-value"},
								{Key: "some-other-bpB-key", Value: "some-other-bpB-value"},
							},
						}, nil)

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}
						if s := cmp.Diff(metadata.Labels, []lifecycle.Label{
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
						bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
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
						bpB.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
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
								Type:        "some-type",
								Command:     "some-command",
								Args:        []string{"some-arg"},
								Direct:      true,
								BuildpackID: "A",
							},
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
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})

					it("should warn when overriding default process type, with non-default process type", func() {
						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							Processes: []launch.Process{
								{
									Type:        "override-with-not-default",
									Command:     "bpA-command",
									Args:        []string{"bpA-arg"},
									Direct:      true,
									BuildpackID: "A",
									Default:     true,
								},
								{
									Type:        "override-with-default",
									Command:     "some-command",
									Args:        []string{"some-arg"},
									Direct:      true,
									BuildpackID: "A",
									Default:     true,
								},
							},
						}, nil)
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							Processes: []launch.Process{
								{
									Type:        "override-with-not-default",
									Command:     "bpB-command",
									Args:        []string{"bpB-arg"},
									Direct:      false,
									BuildpackID: "B",
								},
								{
									Type:        "override-with-default",
									Command:     "some-other-command",
									Args:        []string{"some-other-arg"},
									Direct:      true,
									BuildpackID: "B",
									Default:     true,
								},
							},
						}, nil)

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}
						if s := cmp.Diff(metadata.Processes, []launch.Process{
							{
								Type:        "override-with-not-default",
								Command:     "bpB-command",
								Args:        []string{"bpB-arg"},
								Direct:      false,
								BuildpackID: "B",
								Default:     false,
							},
							{
								Type:        "override-with-default",
								Command:     "some-other-command",
								Args:        []string{"some-other-arg"},
								Direct:      true,
								BuildpackID: "B",
								Default:     true,
							},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}

						expected := "Warning: redefining the following default process types with processes not marked as default: [override-with-not-default]"
						h.AssertStringContains(t, stdout.String(), expected)
					})

					it("should preserve ordering of processes", func() {
						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							Processes: []launch.Process{
								{
									Type:        "first",
									Command:     "first-command",
									Args:        []string{"bpA-arg"},
									Direct:      true,
									BuildpackID: "A",
									Default:     true,
								},
								{
									Type:        "second",
									Command:     "second-command",
									Args:        []string{"some-arg"},
									Direct:      true,
									BuildpackID: "A",
									Default:     true,
								},
							},
						}, nil)
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							Processes: []launch.Process{
								{
									Type:        "third",
									Command:     "third-command",
									Args:        []string{"bpB-arg"},
									Direct:      false,
									BuildpackID: "B",
								},
								{
									Type:        "fourth",
									Command:     "fourth-command",
									Args:        []string{"some-other-arg"},
									Direct:      true,
									BuildpackID: "B",
									Default:     true,
								},
							},
						}, nil)

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}

						if s := cmp.Diff(metadata.Processes, []launch.Process{
							{
								Type:        "first",
								Command:     "first-command",
								Args:        []string{"bpA-arg"},
								Direct:      true,
								BuildpackID: "A",
								Default:     true,
							},
							{
								Type:        "second",
								Command:     "second-command",
								Args:        []string{"some-arg"},
								Direct:      true,
								BuildpackID: "A",
								Default:     true,
							},
							{
								Type:        "third",
								Command:     "third-command",
								Args:        []string{"bpB-arg"},
								Direct:      false,
								BuildpackID: "B",
							},
							{
								Type:        "fourth",
								Command:     "fourth-command",
								Args:        []string{"some-other-arg"},
								Direct:      true,
								BuildpackID: "B",
								Default:     true,
							},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})

					it("shouldn't set web processes as default", func() {
						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							Processes: []launch.Process{
								{
									Type:        "not-web",
									Command:     "not-web-cmd",
									Args:        []string{"not-web-arg"},
									Direct:      true,
									BuildpackID: "A",
									Default:     false,
								},
								{
									Type:        "web",
									Command:     "web-cmd",
									Args:        []string{"web-arg"},
									Direct:      true,
									BuildpackID: "A",
									Default:     false,
								},
							},
						}, nil)
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							Processes: []launch.Process{
								{
									Type:        "another-type",
									Command:     "another-cmd",
									Args:        []string{"another-arg"},
									Direct:      false,
									BuildpackID: "B",
								},
								{
									Type:        "other",
									Command:     "other-cmd",
									Args:        []string{"other-arg"},
									Direct:      true,
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
								Type:        "not-web",
								Command:     "not-web-cmd",
								Args:        []string{"not-web-arg"},
								Direct:      true,
								BuildpackID: "A",
								Default:     false,
							},
							{
								Type:        "web",
								Command:     "web-cmd",
								Args:        []string{"web-arg"},
								Direct:      true,
								BuildpackID: "A",
								Default:     false,
							},
							{
								Type:        "another-type",
								Command:     "another-cmd",
								Args:        []string{"another-arg"},
								Direct:      false,
								BuildpackID: "B",
								Default:     false,
							},
							{
								Type:        "other",
								Command:     "other-cmd",
								Args:        []string{"other-arg"},
								Direct:      true,
								BuildpackID: "B",
								Default:     false,
							},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})

					when("builpack API < 0.6", func() {
						it("sets web processes as default", func() {
							bpA := testmock.NewMockBuildpack(mockCtrl)
							buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
							bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
								Processes: []launch.Process{
									{
										Type:        "some-type",
										Command:     "some-cmd",
										Args:        []string{"some-arg"},
										Direct:      true,
										BuildpackID: "A",
										Default:     false,
									},
									{
										Type:        "another-type",
										Command:     "another-cmd",
										Args:        []string{"another-arg"},
										Direct:      true,
										BuildpackID: "A",
										Default:     false,
									},
								},
							}, nil)
							bpB := testmock.NewMockBuildpack(mockCtrl)
							buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
							bpB.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
								Processes: []launch.Process{
									{
										Type:        "web",
										Command:     "web-command",
										Args:        []string{"web-arg"},
										Direct:      false,
										BuildpackID: "B",
									},
									{
										Type:        "not-web",
										Command:     "not-web-cmd",
										Args:        []string{"not-web-arg"},
										Direct:      true,
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
									Type:        "some-type",
									Command:     "some-cmd",
									Args:        []string{"some-arg"},
									Direct:      true,
									BuildpackID: "A",
									Default:     false,
								},
								{
									Type:        "another-type",
									Command:     "another-cmd",
									Args:        []string{"another-arg"},
									Direct:      true,
									BuildpackID: "A",
									Default:     false,
								},
								{
									Type:        "web",
									Command:     "web-command",
									Args:        []string{"web-arg"},
									Direct:      false,
									BuildpackID: "B",
									Default:     true,
								},
								{
									Type:        "not-web",
									Command:     "not-web-cmd",
									Args:        []string{"not-web-arg"},
									Direct:      true,
									BuildpackID: "B",
									Default:     false,
								},
							}); s != "" {
								t.Fatalf("Unexpected:\n%s\n", s)
							}
						})
					})
				})

				when("slices", func() {
					it("should aggregate slices from each buildpack", func() {
						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							Slices: []layers.Slice{
								{Paths: []string{"some-bpA-path", "some-other-bpA-path"}},
								{Paths: []string{"duplicate-path"}},
								{Paths: []string{"extra-path"}},
							},
						}, nil)
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
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
					bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{}, errors.New("some error"))

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
					bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{}, nil)
					bpB := testmock.NewMockBuildpack(mockCtrl)
					buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
					bpB.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{}, errors.New("some error"))

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
				builder.PlatformAPI = api.MustParse("0.3")
			})

			when("build metadata", func() {
				when("bom", func() {
					it("should convert metadata.version to top level version", func() {
						bpA := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("A", "v1").Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							BOM: []lifecycle.BOMEntry{
								{
									Require: lifecycle.Require{
										Name:     "dep1",
										Metadata: map[string]interface{}{"version": string("v1")},
									},
									Buildpack: lifecycle.GroupBuildpack{ID: "A", Version: "v1"},
								},
							},
						}, nil)
						bpB := testmock.NewMockBuildpack(mockCtrl)
						buildpackStore.EXPECT().Lookup("B", "v2").Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config)

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}

						if s := cmp.Diff(metadata.BOM, []lifecycle.BOMEntry{
							{
								Require: lifecycle.Require{
									Name:     "dep1",
									Version:  "v1",
									Metadata: map[string]interface{}{"version": string("v1")},
								},
								Buildpack: lifecycle.GroupBuildpack{ID: "A", Version: "v1"},
							},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})
				})
			})
		})
	})
}
