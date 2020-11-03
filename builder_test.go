package lifecycle_test

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
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/testmock"
)

const latestPlatformAPI = "0.5" // TODO: is there a good way to ensure this is kept up to date?

func TestBuilder(t *testing.T) {
	spec.Run(t, "Builder", testBuilder, spec.Report(report.Terminal{}))
}

//go:generate mockgen -package testmock -destination testmock/env.go github.com/buildpacks/lifecycle BuildEnv
//go:generate mockgen -package testmock -destination testmock/buildpack_finder.go github.com/buildpacks/lifecycle BuildpackFinder
//go:generate mockgen -package testmock -destination testmock/buildpack_toml.go github.com/buildpacks/lifecycle BuildpackTOML

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		builder         *lifecycle.Builder
		mockCtrl        *gomock.Controller
		env             *testmock.MockBuildEnv
		buildpackFinder *testmock.MockBuildpackFinder
		stdout, stderr  *bytes.Buffer
		tmpDir          string
		platformDir     string
		appDir          string
		layersDir       string
		config          lifecycle.BuildConfig
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		env = testmock.NewMockBuildEnv(mockCtrl)
		buildpackFinder = testmock.NewMockBuildpackFinder(mockCtrl)

		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
		platformDir = filepath.Join(tmpDir, "platform")
		layersDir = filepath.Join(tmpDir, "launch")
		appDir = filepath.Join(layersDir, "app")
		mkdir(t, layersDir, appDir, filepath.Join(platformDir, "env"))

		builder = &lifecycle.Builder{
			AppDir:        appDir,
			LayersDir:     layersDir,
			PlatformDir:   platformDir,
			BuildpacksDir: filepath.Join("testdata", "by-id"),
			PlatformAPI:   api.MustParse(latestPlatformAPI),
			Env:           env,
			Group: lifecycle.BuildpackGroup{
				Group: []lifecycle.Buildpack{
					{ID: "A", Version: "v1", API: "0.5", Homepage: "Buildpack A Homepage"},
					{ID: "B", Version: "v2", API: "0.2"},
				},
			},
			Out:             stdout,
			Err:             stderr,
			BuildpackFinder: buildpackFinder,
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
							Providers: []lifecycle.Buildpack{
								{ID: "A", Version: "v1"},
								{ID: "B", Version: "v2"},
							},
							Requires: []lifecycle.Require{
								{Name: "some-dep", Version: "v1"}, // not provided to buildpack B because it is met
							},
						},
						{
							Providers: []lifecycle.Buildpack{
								{ID: "A", Version: "v1"},
								{ID: "B", Version: "v2"},
							},
							Requires: []lifecycle.Require{
								{Name: "some-unmet-dep", Version: "v2"}, // provided to buildpack B because it is unmet
							},
						},
						{
							Providers: []lifecycle.Buildpack{
								{ID: "B", Version: "v2"},
							},
							Requires: []lifecycle.Require{
								{Name: "other-dep", Version: "v4"}, // only provided to buildpack B
							},
						},
					},
				}
				bpA := testmock.NewMockBuildpackTOML(mockCtrl)
				bpB := testmock.NewMockBuildpackTOML(mockCtrl)
				buildpackFinder.EXPECT().Find("A", "v1", builder.BuildpacksDir).Return(bpA, nil)
				expectedPlanA := lifecycle.BuildpackPlan{Entries: []lifecycle.Require{
					{Name: "some-dep", Version: "v1"},
					{Name: "some-unmet-dep", Version: "v2"},
				}}
				bpA.EXPECT().Build(expectedPlanA, config).Return(lifecycle.BuildResult{
					Met: []string{"some-dep"},
				}, nil)
				buildpackFinder.EXPECT().Find("B", "v2", builder.BuildpacksDir).Return(bpB, nil)
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
					it("should convert top level version to metadata.version", func() {
						bpA := testmock.NewMockBuildpackTOML(mockCtrl)
						buildpackFinder.EXPECT().Find("A", "v1", builder.BuildpacksDir).Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							BOM: []lifecycle.BOMEntry{
								{
									Require: lifecycle.Require{
										Name:    "dep1",
										Version: "v1",
									},
									Buildpack: lifecycle.Buildpack{ID: "A", Version: "v1"},
								},
							},
						}, nil)
						bpB := testmock.NewMockBuildpackTOML(mockCtrl)
						buildpackFinder.EXPECT().Find("B", "v2", builder.BuildpacksDir).Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config)

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
								Buildpack: lifecycle.Buildpack{ID: "A", Version: "v1"},
							},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})
				})

				when("buildpacks", func() {
					it("should include builder buildpacks", func() {
						bpA := testmock.NewMockBuildpackTOML(mockCtrl)
						buildpackFinder.EXPECT().Find("A", "v1", builder.BuildpacksDir).Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config)
						bpB := testmock.NewMockBuildpackTOML(mockCtrl)
						buildpackFinder.EXPECT().Find("B", "v2", builder.BuildpacksDir).Return(bpB, nil)
						bpB.EXPECT().Build(gomock.Any(), config)

						metadata, err := builder.Build()
						if err != nil {
							t.Fatalf("Unexpected error:\n%s\n", err)
						}
						if s := cmp.Diff(metadata.Buildpacks, []lifecycle.Buildpack{
							{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"},
							{ID: "B", Version: "v2", API: "0.2"},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
					})
				})

				when("labels", func() {
					it("should aggregate labels from each buildpack", func() {
						bpA := testmock.NewMockBuildpackTOML(mockCtrl)
						buildpackFinder.EXPECT().Find("A", "v1", builder.BuildpacksDir).Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							Labels: []lifecycle.Label{
								{Key: "some-bpA-key", Value: "some-bpA-value"},
								{Key: "some-other-bpA-key", Value: "some-other-bpA-value"},
							},
						}, nil)
						bpB := testmock.NewMockBuildpackTOML(mockCtrl)
						buildpackFinder.EXPECT().Find("B", "v2", builder.BuildpacksDir).Return(bpB, nil)
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
						bpA := testmock.NewMockBuildpackTOML(mockCtrl)
						buildpackFinder.EXPECT().Find("A", "v1", builder.BuildpacksDir).Return(bpA, nil)
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
						bpB := testmock.NewMockBuildpackTOML(mockCtrl)
						buildpackFinder.EXPECT().Find("B", "v2", builder.BuildpacksDir).Return(bpB, nil)
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
					})
				})

				when("slices", func() {
					it("should aggregate slices from each buildpack", func() {
						bpA := testmock.NewMockBuildpackTOML(mockCtrl)
						buildpackFinder.EXPECT().Find("A", "v1", builder.BuildpacksDir).Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							Slices: []layers.Slice{
								{Paths: []string{"some-bpA-path", "some-other-bpA-path"}},
								{Paths: []string{"duplicate-path"}},
								{Paths: []string{"extra-path"}},
							},
						}, nil)
						bpB := testmock.NewMockBuildpackTOML(mockCtrl)
						buildpackFinder.EXPECT().Find("B", "v2", builder.BuildpacksDir).Return(bpB, nil)
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
					bpA := testmock.NewMockBuildpackTOML(mockCtrl)
					buildpackFinder.EXPECT().Find("A", "v1", builder.BuildpacksDir).Return(bpA, nil)
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
					bpA := testmock.NewMockBuildpackTOML(mockCtrl)
					buildpackFinder.EXPECT().Find("A", "v1", builder.BuildpacksDir).Return(bpA, nil)
					bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{}, nil)
					bpB := testmock.NewMockBuildpackTOML(mockCtrl)
					buildpackFinder.EXPECT().Find("B", "v2", builder.BuildpacksDir).Return(bpB, nil)
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
						bpA := testmock.NewMockBuildpackTOML(mockCtrl)
						buildpackFinder.EXPECT().Find("A", "v1", builder.BuildpacksDir).Return(bpA, nil)
						bpA.EXPECT().Build(gomock.Any(), config).Return(lifecycle.BuildResult{
							BOM: []lifecycle.BOMEntry{
								{
									Require: lifecycle.Require{
										Name:     "dep1",
										Metadata: map[string]interface{}{"version": string("v1")},
									},
									Buildpack: lifecycle.Buildpack{ID: "A", Version: "v1"},
								},
							},
						}, nil)
						bpB := testmock.NewMockBuildpackTOML(mockCtrl)
						buildpackFinder.EXPECT().Find("B", "v2", builder.BuildpacksDir).Return(bpB, nil)
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
								Buildpack: lifecycle.Buildpack{ID: "A", Version: "v1"},
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

func mkdir(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0777); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func mkfile(t *testing.T, data string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := ioutil.WriteFile(p, []byte(data), 0777); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func tofile(t *testing.T, data string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0777)
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		if _, err := f.Write([]byte(data)); err != nil {
			f.Close()
			t.Fatalf("Error: %s\n", err)
		}
		f.Close()
	}
}

func cleanEndings(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func rdfile(t *testing.T, path string) string {
	t.Helper()
	out, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("Error: %s\n", err)
	}
	return cleanEndings(string(out))
}

func testExists(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func testPlan(t *testing.T, plan []lifecycle.Require, paths ...string) {
	t.Helper()
	for _, p := range paths {
		var c struct {
			Entries []lifecycle.Require `toml:"entries"`
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
