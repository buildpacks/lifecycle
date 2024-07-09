package phase_test

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
	llog "github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/phase/testmock"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestBuilder(t *testing.T) {
	spec.Run(t, "Builder", testBuilder, spec.Report(report.Terminal{}))
}

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		mockCtrl       *gomock.Controller
		builder        *phase.Builder
		tmpDir         string
		appDir         string
		layersDir      string
		platformDir    string
		dirStore       *testmock.MockDirStore
		executor       *testmock.MockBuildExecutor
		logHandler     = memory.New()
		stdout, stderr *bytes.Buffer
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		dirStore = testmock.NewMockDirStore(mockCtrl)
		executor = testmock.NewMockBuildExecutor(mockCtrl)

		var err error
		tmpDir, err = os.MkdirTemp("", "lifecycle")
		h.AssertNil(t, err)
		layersDir = filepath.Join(tmpDir, "launch")
		appDir = filepath.Join(layersDir, "app")
		platformDir = filepath.Join(tmpDir, "platform")
		h.Mkdir(t, layersDir, appDir, filepath.Join(platformDir, "env"))
		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}

		builder = &phase.Builder{
			AppDir:        appDir,
			LayersDir:     layersDir,
			PlatformDir:   platformDir,
			BuildExecutor: executor,
			DirStore:      dirStore,
			Group: buildpack.Group{
				Group: []buildpack.GroupElement{
					{ID: "A", Version: "v1", API: api.Buildpack.Latest().String(), Homepage: "Buildpack A Homepage"},
					{ID: "B", Version: "v2", API: api.Buildpack.Latest().String()},
				},
			},
			Logger:      &log.Logger{Handler: logHandler},
			Out:         stdout,
			Err:         stderr,
			PlatformAPI: api.Platform.Latest(),
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
		mockCtrl.Finish()
	})

	when("#Build", func() {
		it("cleans the /layers/sbom directory before building", func() {
			oldDir := filepath.Join(layersDir, "sbom", "launch", "undetected-buildpack")
			h.Mkdir(t, oldDir)
			oldFile := filepath.Join(oldDir, "launch.sbom.cdx.json")
			h.Mkfile(t, `{"key": "some-bom-content"}`, oldFile)
			bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
			bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
			dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
			executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{}, nil)
			executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{}, nil)

			_, err := builder.Build()
			h.AssertNil(t, err)

			h.AssertPathDoesNotExist(t, oldFile)
		})

		it("provides a subset of the build plan to each buildpack", func() {
			builder.Plan = files.Plan{
				Entries: []files.BuildPlanEntry{
					{
						Providers: []buildpack.GroupElement{
							{ID: "A", Version: "v1", Extension: true}, // not provided to any buildpack
						},
						Requires: []buildpack.Require{
							{Name: "extension-dep", Version: "v1"},
						},
					},
					{
						Providers: []buildpack.GroupElement{
							{ID: "A", Version: "v1"},
							{ID: "B", Version: "v2"},
						},
						Requires: []buildpack.Require{
							{Name: "some-dep", Version: "v1"}, // not provided to buildpack B because it is met
						},
					},
					{
						Providers: []buildpack.GroupElement{
							{ID: "A", Version: "v1"},
							{ID: "B", Version: "v2"},
						},
						Requires: []buildpack.Require{
							{Name: "some-unmet-dep", Version: "v2"}, // provided to buildpack B because it is unmet
						},
					},
					{
						Providers: []buildpack.GroupElement{
							{ID: "B", Version: "v2"},
						},
						Requires: []buildpack.Require{
							{Name: "other-dep", Version: "v4"}, // only provided to buildpack B
						},
					},
				},
			}
			bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
			bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
			expectedPlanA := buildpack.Plan{Entries: []buildpack.Require{
				{Name: "some-dep", Version: "v1"},
				{Name: "some-unmet-dep", Version: "v2"},
			}}
			executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ buildpack.BpDescriptor, inputs buildpack.BuildInputs, _ llog.Logger) (buildpack.BuildOutputs, error) {
					h.AssertEq(t, inputs.AppDir, builder.AppDir)
					h.AssertEq(t, inputs.BuildConfigDir, builder.BuildConfigDir)
					h.AssertEq(t, inputs.PlatformDir, builder.PlatformDir)
					h.AssertEq(t, inputs.Plan, expectedPlanA)
					return buildpack.BuildOutputs{
						MetRequires: []string{"some-dep"},
					}, nil
				})
			dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
			expectedPlanB := buildpack.Plan{Entries: []buildpack.Require{
				{Name: "some-unmet-dep", Version: "v2"},
				{Name: "other-dep", Version: "v4"},
			}}
			executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Do(
				func(_ buildpack.BpDescriptor, inputs buildpack.BuildInputs, _ llog.Logger) (buildpack.BuildOutputs, error) {
					h.AssertEq(t, inputs.AppDir, builder.AppDir)
					h.AssertEq(t, inputs.BuildConfigDir, builder.BuildConfigDir)
					h.AssertEq(t, inputs.PlatformDir, builder.PlatformDir)
					h.AssertEq(t, inputs.Plan, expectedPlanB)
					return buildpack.BuildOutputs{}, nil
				})

			_, err := builder.Build()
			h.AssertNil(t, err)
		})

		it("gets the correct env vars", func() {
			builder.AnalyzeMD.RunImage = &files.RunImage{Reference: "foo", TargetMetadata: &files.TargetMetadata{
				OS:   "linux",
				Arch: "amd64",
			}}

			bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}

			executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ buildpack.BpDescriptor, inputs buildpack.BuildInputs, logger llog.Logger) (buildpack.BuildOutputs, error) {
					h.AssertContains(t, inputs.TargetEnv, "CNB_TARGET_ARCH=amd64")
					h.AssertContains(t, inputs.TargetEnv, "CNB_TARGET_OS=linux")
					return buildpack.BuildOutputs{}, nil
				},
			)
			bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}

			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
			dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
			executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Do(
				func(_ buildpack.BpDescriptor, inputs buildpack.BuildInputs, _ llog.Logger) (buildpack.BuildOutputs, error) {
					h.AssertContains(t, inputs.TargetEnv, "CNB_TARGET_ARCH=amd64")
					h.AssertContains(t, inputs.TargetEnv, "CNB_TARGET_OS=linux")
					return buildpack.BuildOutputs{}, nil
				})

			_, err := builder.Build()
			h.AssertNil(t, err)
		})

		it("doesnt gets the new env vars if its old", func() {
			builder.PlatformAPI = api.MustParse("0.8")
			builder.AnalyzeMD.RunImage = &files.RunImage{Reference: "foo", TargetMetadata: &files.TargetMetadata{
				OS:   "linux",
				Arch: "amd64",
			}}

			bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}

			executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ buildpack.BpDescriptor, inputs buildpack.BuildInputs, logger llog.Logger) (buildpack.BuildOutputs, error) {
					h.AssertDoesNotContain(t, inputs.Env.List(), "CNB_TARGET_ARCH=amd64")
					h.AssertDoesNotContain(t, inputs.Env.List(), "CNB_TARGET_OS=linux")
					h.AssertDoesNotContain(t, inputs.Env.List(), "CNB_TARGET_VARIANT=")
					h.AssertDoesNotContain(t, inputs.Env.List(), "CNB_TARGET_DISTRO_NAME=")
					h.AssertDoesNotContain(t, inputs.Env.List(), "CNB_TARGET_DISTRO_VERSION=")
					return buildpack.BuildOutputs{}, nil
				},
			)
			bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}

			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
			dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
			executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Do(
				func(_ buildpack.BpDescriptor, inputs buildpack.BuildInputs, _ llog.Logger) (buildpack.BuildOutputs, error) {
					h.AssertDoesNotContain(t, inputs.Env.List(), "CNB_TARGET_ARCH=amd64")
					h.AssertDoesNotContain(t, inputs.Env.List(), "CNB_TARGET_OS=linux")
					h.AssertDoesNotContain(t, inputs.Env.List(), "CNB_TARGET_VARIANT=")
					h.AssertDoesNotContain(t, inputs.Env.List(), "CNB_TARGET_DISTRO_NAME=")
					h.AssertDoesNotContain(t, inputs.Env.List(), "CNB_TARGET_DISTRO_VERSION=")
					return buildpack.BuildOutputs{}, nil
				})

			_, err := builder.Build()
			h.AssertNil(t, err)
		})

		it("provides the updated environment to the next buildpack", func() {
			bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}

			executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ buildpack.BpDescriptor, inputs buildpack.BuildInputs, logger llog.Logger) (buildpack.BuildOutputs, error) {
					envPtr := inputs.Env.(*env.Env)
					newEnv := env.NewBuildEnv(append(os.Environ(), "HOME=some-val-from-bpA"))
					*(envPtr) = *newEnv // modify the provided env
					return buildpack.BuildOutputs{}, nil
				},
			)
			bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}

			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
			dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
			executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Do(
				func(_ buildpack.BpDescriptor, inputs buildpack.BuildInputs, _ llog.Logger) (buildpack.BuildOutputs, error) {
					h.AssertContains(t, inputs.Env.List(), "HOME=some-val-from-bpA")
					return buildpack.BuildOutputs{}, nil
				})

			_, err := builder.Build()
			h.AssertNil(t, err)
		})

		it("copies SBOM files to the correct locations", func() {
			bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
			bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
			dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)

			bomFilePath1 := filepath.Join(layersDir, "launch.sbom.cdx.json")
			bomFilePath2 := filepath.Join(layersDir, "build.sbom.cdx.json")
			bomFilePath3 := filepath.Join(layersDir, "layer-b1.sbom.cdx.json")
			bomFilePath4 := filepath.Join(layersDir, "layer-b2.sbom.cdx.json")
			h.Mkfile(t, `{"key": "some-bom-content-1"}`, bomFilePath1)
			h.Mkfile(t, `{"key": "some-bom-content-2"}`, bomFilePath2)
			h.Mkfile(t, `{"key": "some-bom-content-3"}`, bomFilePath3)
			h.Mkfile(t, `{"key": "some-bom-content-4"}`, bomFilePath4)

			executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
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
			executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
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
			h.AssertNil(t, err)

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

		it("errors if there are any unsupported SBOM formats", func() {
			bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
			bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
			dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)

			bomFilePath1 := filepath.Join(layersDir, "launch.sbom.cdx.json")
			bomFilePath2 := filepath.Join(layersDir, "layer-b.sbom.some-unknown-format.json")
			h.Mkfile(t, `{"key": "some-bom-content-a"}`, bomFilePath1)
			h.Mkfile(t, `{"key": "some-bom-content-b"}`, bomFilePath2)

			executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
				BOMFiles: []buildpack.BOMFile{
					{
						BuildpackID: "A",
						LayerName:   "",
						LayerType:   buildpack.LayerTypeLaunch,
						Path:        bomFilePath1,
					},
				},
			}, nil)
			executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
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
			h.AssertError(t, err, fmt.Sprintf("unsupported SBOM format: '%s'", bomFilePath2))
		})

		when("build metadata", func() {
			when("bom", func() {
				it("omits bom and saves the aggregated legacy boms to <layers>/sbom/", func() {
					builder.Group.Group = []buildpack.GroupElement{
						{ID: "A", Version: "v1", Homepage: "Buildpack A Homepage"},
						{ID: "B", Version: "v2"},
					}

					bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
					executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
						BuildBOM: []buildpack.BOMEntry{
							{
								Require: buildpack.Require{
									Name:     "build-dep1",
									Metadata: map[string]interface{}{"version": "v1"},
								},
								Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"},
							},
						},
						LaunchBOM: []buildpack.BOMEntry{
							{
								Require: buildpack.Require{
									Name:     "launch-dep1",
									Metadata: map[string]interface{}{"version": "v1"},
								},
								Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"},
							},
						},
					}, nil)
					bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
					executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
						BuildBOM: []buildpack.BOMEntry{
							{
								Require: buildpack.Require{
									Name:     "build-dep2",
									Metadata: map[string]interface{}{"version": "v1"},
								},
								Buildpack: buildpack.GroupElement{ID: "B", Version: "v2"},
							},
						},
						LaunchBOM: []buildpack.BOMEntry{
							{
								Require: buildpack.Require{
									Name:     "launch-dep2",
									Metadata: map[string]interface{}{"version": "v1"},
								},
								Buildpack: buildpack.GroupElement{ID: "B", Version: "v2"},
							},
						},
					}, nil)

					metadata, err := builder.Build()
					h.AssertNil(t, err)
					if s := cmp.Diff(metadata.BOM, []buildpack.BOMEntry{}); s != "" {
						t.Fatalf("Unexpected:\n%s\n", s)
					}

					t.Log("saves the aggregated legacy launch bom to <layers>/sbom/launch/sbom.legacy.json")
					var foundLaunch []buildpack.BOMEntry
					launchContents, err := os.ReadFile(filepath.Join(builder.LayersDir, "sbom", "launch", "sbom.legacy.json"))
					h.AssertNil(t, err)
					h.AssertNil(t, json.Unmarshal(launchContents, &foundLaunch))
					expectedLaunch := []buildpack.BOMEntry{
						{
							Require: buildpack.Require{
								Name:     "launch-dep1",
								Version:  "",
								Metadata: map[string]interface{}{"version": string("v1")},
							},
							Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"},
						},
						{
							Require: buildpack.Require{
								Name:     "launch-dep2",
								Version:  "",
								Metadata: map[string]interface{}{"version": string("v1")},
							},
							Buildpack: buildpack.GroupElement{ID: "B", Version: "v2"},
						},
					}
					h.AssertEq(t, foundLaunch, expectedLaunch)

					t.Log("saves the aggregated legacy build bom to <layers>/sbom/build/sbom.legacy.json")
					var foundBuild []buildpack.BOMEntry
					buildContents, err := os.ReadFile(filepath.Join(builder.LayersDir, "sbom", "build", "sbom.legacy.json"))
					h.AssertNil(t, err)
					h.AssertNil(t, json.Unmarshal(buildContents, &foundBuild))
					expectedBuild := []buildpack.BOMEntry{
						{
							Require: buildpack.Require{
								Name:     "build-dep1",
								Version:  "",
								Metadata: map[string]interface{}{"version": string("v1")},
							},
							Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"},
						},
						{
							Require: buildpack.Require{
								Name:     "build-dep2",
								Version:  "",
								Metadata: map[string]interface{}{"version": string("v1")},
							},
							Buildpack: buildpack.GroupElement{ID: "B", Version: "v2"},
						},
					}
					h.AssertEq(t, foundBuild, expectedBuild)
				})
			})

			when("buildpacks", func() {
				it.Before(func() {
					bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
					executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any())
					bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
					executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any())
				})

				it("includes the provided buildpacks with homepage information", func() {
					metadata, err := builder.Build()
					h.AssertNil(t, err)
					h.AssertEq(t, metadata.Buildpacks, []buildpack.GroupElement{
						{ID: "A", Version: "v1", API: api.Buildpack.Latest().String(), Homepage: "Buildpack A Homepage"},
						{ID: "B", Version: "v2", API: api.Buildpack.Latest().String()},
					})
				})
			})

			when("extensions", func() {
				it.Before(func() {
					bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
					executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any())
					bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
					executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any())
				})

				it("includes the provided extensions with homepage information", func() {
					builder.Group.GroupExtensions = []buildpack.GroupElement{
						{ID: "A", Version: "v1", API: "0.9", Homepage: "some-homepage", Extension: true, Optional: true},
					}
					metadata, err := builder.Build()
					h.AssertNil(t, err)
					h.AssertEq(t, metadata.Extensions, []buildpack.GroupElement{
						{ID: "A", Version: "v1", API: "0.9", Homepage: "some-homepage"}, // this shows that `extension = true` and `optional = true` are not redundantly printed in metadata.toml
					})
				})
			})

			when("labels", func() {
				it("aggregates labels from each buildpack", func() {
					bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
					executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
						Labels: []buildpack.Label{
							{Key: "some-bpA-key", Value: "some-bpA-value"},
							{Key: "some-other-bpA-key", Value: "some-other-bpA-value"},
						},
					}, nil)
					bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
					executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
						Labels: []buildpack.Label{
							{Key: "some-bpB-key", Value: "some-bpB-value"},
							{Key: "some-other-bpB-key", Value: "some-other-bpB-value"},
						},
					}, nil)

					metadata, err := builder.Build()
					h.AssertNil(t, err)
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
				it("overrides identical processes from earlier buildpacks", func() {
					bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
					executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
						Processes: []launch.Process{
							{
								Type:        "some-type",
								Command:     launch.NewRawCommand([]string{"some-command"}),
								Args:        []string{"some-arg"},
								Direct:      true,
								BuildpackID: "A",
							},
							{
								Type:        "override-type",
								Command:     launch.NewRawCommand([]string{"bpA-command"}),
								Args:        []string{"bpA-arg"},
								Direct:      true,
								BuildpackID: "A",
							},
						},
					}, nil)
					bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
					executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
						Processes: []launch.Process{
							{
								Type:        "some-other-type",
								Command:     launch.NewRawCommand([]string{"some-other-command"}),
								Args:        []string{"some-other-arg"},
								Direct:      true,
								BuildpackID: "B",
							},
							{
								Type:        "override-type",
								Command:     launch.NewRawCommand([]string{"bpB-command"}),
								Args:        []string{"bpB-arg"},
								Direct:      false,
								BuildpackID: "B",
							},
						},
					}, nil)

					metadata, err := builder.Build()
					h.AssertNil(t, err)
					if s := cmp.Diff(metadata.Processes, []launch.Process{
						{
							Type: "override-type",
							Command: launch.NewRawCommand([]string{"bpB-command"}).
								WithPlatformAPI(builder.PlatformAPI),
							Args:        []string{"bpB-arg"},
							Direct:      false,
							BuildpackID: "B",
							PlatformAPI: builder.PlatformAPI,
						},
						{
							Type: "some-other-type",
							Command: launch.NewRawCommand([]string{"some-other-command"}).
								WithPlatformAPI(builder.PlatformAPI),
							Args:        []string{"some-other-arg"},
							Direct:      true,
							BuildpackID: "B",
							PlatformAPI: builder.PlatformAPI,
						},
						{
							Type: "some-type",
							Command: launch.NewRawCommand([]string{"some-command"}).
								WithPlatformAPI(builder.PlatformAPI),
							Args:        []string{"some-arg"},
							Direct:      true,
							BuildpackID: "A",
							PlatformAPI: builder.PlatformAPI,
						},
					}); s != "" {
						t.Fatalf("Unexpected:\n%s\n", s)
					}
					h.AssertEq(t, metadata.BuildpackDefaultProcessType, "")
				})

				when("multiple default process types", func() {
					it.Before(func() {
						builder.Group.Group = []buildpack.GroupElement{
							{ID: "A", Version: "v1", API: api.Buildpack.Latest().String()},
							{ID: "B", Version: "v2", API: api.Buildpack.Latest().String()},
							{ID: "C", Version: "v3", API: api.Buildpack.Latest().String()},
						}
					})

					it("picks the last default process type", func() {
						bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
						dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
						executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
							Processes: []launch.Process{
								{
									Type:        "override-type",
									Command:     launch.NewRawCommand([]string{"bpA-command"}),
									Args:        []string{"bpA-arg"},
									Direct:      true,
									BuildpackID: "A",
									Default:     true,
								},
							},
						}, nil)
						bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
						dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
						executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
							Processes: []launch.Process{
								{
									Type:        "some-type",
									Command:     launch.NewRawCommand([]string{"bpB-command"}),
									Args:        []string{"bpB-arg"},
									Direct:      false,
									BuildpackID: "B",
									Default:     true,
								},
							},
						}, nil)

						bpC := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "C", Version: "v3"}}}
						dirStore.EXPECT().LookupBp("C", "v3").Return(bpC, nil)
						executor.EXPECT().Build(*bpC, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
							Processes: []launch.Process{
								{
									Type:        "override-type",
									Command:     launch.NewRawCommand([]string{"bpC-command"}),
									Args:        []string{"bpC-arg"},
									Direct:      false,
									BuildpackID: "C",
								},
							},
						}, nil)

						metadata, err := builder.Build()
						h.AssertNil(t, err)

						if s := cmp.Diff(metadata.Processes, []launch.Process{
							{
								Type: "override-type",
								Command: launch.NewRawCommand([]string{"bpC-command"}).
									WithPlatformAPI(builder.PlatformAPI),
								Args:        []string{"bpC-arg"},
								Direct:      false,
								BuildpackID: "C",
								PlatformAPI: builder.PlatformAPI,
							},
							{
								Type: "some-type",
								Command: launch.NewRawCommand([]string{"bpB-command"}).
									WithPlatformAPI(builder.PlatformAPI),
								Args:        []string{"bpB-arg"},
								Direct:      false,
								BuildpackID: "B",
								PlatformAPI: builder.PlatformAPI,
							},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
						h.AssertEq(t, metadata.BuildpackDefaultProcessType, "some-type")
					})
				})

				when("overriding default process type, with a non-default process type", func() {
					it.Before(func() {
						builder.Group.Group = []buildpack.GroupElement{
							{ID: "A", Version: "v1", API: api.Buildpack.Latest().String()},
							{ID: "B", Version: "v2", API: api.Buildpack.Latest().String()},
							{ID: "C", Version: "v3", API: api.Buildpack.Latest().String()},
						}
					})

					it("warns and does not set any default process", func() {
						bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
						dirStore.EXPECT().LookupBp("A", "v1").Return(bpB, nil)
						executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
							Processes: []launch.Process{
								{
									Type:        "some-type",
									Command:     launch.NewRawCommand([]string{"bpA-command"}),
									Args:        []string{"bpA-arg"},
									Direct:      false,
									BuildpackID: "A",
									Default:     true,
								},
							},
						}, nil)

						bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
						dirStore.EXPECT().LookupBp("B", "v2").Return(bpA, nil)
						executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
							Processes: []launch.Process{
								{
									Type:        "override-type",
									Command:     launch.NewRawCommand([]string{"bpB-command"}),
									Args:        []string{"bpB-arg"},
									Direct:      true,
									BuildpackID: "B",
									Default:     true,
								},
							},
						}, nil)

						bpC := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "C", Version: "v3"}}}
						dirStore.EXPECT().LookupBp("C", "v3").Return(bpC, nil)
						executor.EXPECT().Build(*bpC, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
							Processes: []launch.Process{
								{
									Type:        "override-type",
									Command:     launch.NewRawCommand([]string{"bpC-command"}),
									Args:        []string{"bpC-arg"},
									Direct:      false,
									BuildpackID: "C",
								},
							},
						}, nil)

						metadata, err := builder.Build()
						h.AssertNil(t, err)
						if s := cmp.Diff(metadata.Processes, []launch.Process{
							{
								Type: "override-type",
								Command: launch.NewRawCommand([]string{"bpC-command"}).
									WithPlatformAPI(builder.PlatformAPI),
								Args:        []string{"bpC-arg"},
								Direct:      false,
								BuildpackID: "C",
								PlatformAPI: builder.PlatformAPI,
							},
							{
								Type: "some-type",
								Command: launch.NewRawCommand([]string{"bpA-command"}).
									WithPlatformAPI(builder.PlatformAPI),
								Args:        []string{"bpA-arg"},
								Direct:      false,
								BuildpackID: "A",
								PlatformAPI: builder.PlatformAPI,
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
					it.Before(func() {
						builder.Group.Group = []buildpack.GroupElement{
							{ID: "A", Version: "v1", API: api.Buildpack.Latest().String()},
						}
					})

					it("does not set it as a default process", func() {
						bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
						dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
						executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
							Processes: []launch.Process{
								{
									Type:        "web",
									Command:     launch.NewRawCommand([]string{"web-cmd"}),
									Args:        []string{"web-arg"},
									Direct:      false,
									BuildpackID: "A",
									Default:     false,
								},
							},
						}, nil)

						metadata, err := builder.Build()
						h.AssertNil(t, err)

						if s := cmp.Diff(metadata.Processes, []launch.Process{
							{
								Type: "web",
								Command: launch.NewRawCommand([]string{"web-cmd"}).
									WithPlatformAPI(builder.PlatformAPI),
								Args:        []string{"web-arg"},
								Direct:      false,
								BuildpackID: "A",
								Default:     false,
								PlatformAPI: builder.PlatformAPI,
							},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}
						h.AssertEq(t, metadata.BuildpackDefaultProcessType, "")
					})
				})

				it("includes the platform API version", func() {
					builder.Group.Group = []buildpack.GroupElement{
						{ID: "A", Version: "v1", API: api.Buildpack.Latest().String()},
					}
					bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
					executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
						Processes: []launch.Process{
							{
								Type:        "some-type",
								Command:     launch.NewRawCommand([]string{"some-cmd"}),
								BuildpackID: "A",
							},
						},
					}, nil)

					metadata, err := builder.Build()
					h.AssertNil(t, err)

					h.AssertEq(t, metadata.Processes[0].PlatformAPI, builder.PlatformAPI)
				})
			})

			when("slices", func() {
				it("aggregates slices from each buildpack", func() {
					bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
					executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
						Slices: []layers.Slice{
							{Paths: []string{"some-bpA-path", "some-other-bpA-path"}},
							{Paths: []string{"duplicate-path"}},
							{Paths: []string{"extra-path"}},
						},
					}, nil)
					bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
					executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
						Slices: []layers.Slice{
							{Paths: []string{"some-bpB-path", "some-other-bpB-path"}},
							{Paths: []string{"duplicate-path"}},
						},
					}, nil)

					metadata, err := builder.Build()
					h.AssertNil(t, err)
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

		when("buildpack build fails", func() {
			when("first buildpack build fails", func() {
				it("errors", func() {
					bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
					executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{}, errors.New("some error"))

					if _, err := builder.Build(); err == nil {
						t.Fatal("Expected error.\n")
					} else if !strings.Contains(err.Error(), "some error") {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})
			})

			when("later buildpack build fails", func() {
				it("errors", func() {
					bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
					executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{}, nil)
					bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
					dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
					executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{}, errors.New("some error"))

					if _, err := builder.Build(); err == nil {
						t.Fatal("Expected error.\n")
					} else if !strings.Contains(err.Error(), "some error") {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})
			})
		})

		when("platform api < 0.9", func() {
			it.Before(func() {
				builder.PlatformAPI = api.MustParse("0.8")
			})

			when("build metadata", func() {
				when("bom", func() {
					it("aggregates boms from each buildpack", func() {
						builder.Group.Group = []buildpack.GroupElement{
							{ID: "A", Version: "v1", API: "0.5", Homepage: "Buildpack A Homepage"},
							{ID: "B", Version: "v2", API: "0.2"},
						}

						bpA := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
						dirStore.EXPECT().LookupBp("A", "v1").Return(bpA, nil)
						executor.EXPECT().Build(*bpA, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
							LaunchBOM: []buildpack.BOMEntry{
								{
									Require: buildpack.Require{
										Name:     "dep1",
										Metadata: map[string]interface{}{"version": "v1"},
									},
									Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"},
								},
							},
						}, nil)
						bpB := &buildpack.BpDescriptor{Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
						dirStore.EXPECT().LookupBp("B", "v2").Return(bpB, nil)
						executor.EXPECT().Build(*bpB, gomock.Any(), gomock.Any()).Return(buildpack.BuildOutputs{
							LaunchBOM: []buildpack.BOMEntry{
								{
									Require: buildpack.Require{
										Name:     "dep2",
										Metadata: map[string]interface{}{"version": "v1"},
									},
									Buildpack: buildpack.GroupElement{ID: "B", Version: "v2"},
								},
							},
						}, nil)

						metadata, err := builder.Build()
						h.AssertNil(t, err)

						if s := cmp.Diff(metadata.BOM, []buildpack.BOMEntry{
							{
								Require: buildpack.Require{
									Name:     "dep1",
									Version:  "",
									Metadata: map[string]interface{}{"version": string("v1")},
								},
								Buildpack: buildpack.GroupElement{ID: "A", Version: "v1"},
							},
							{
								Require: buildpack.Require{
									Name:     "dep2",
									Version:  "",
									Metadata: map[string]interface{}{"version": string("v1")},
								},
								Buildpack: buildpack.GroupElement{ID: "B", Version: "v2"},
							},
						}); s != "" {
							t.Fatalf("Unexpected:\n%s\n", s)
						}

						t.Log("it does not save the aggregated legacy launch bom to <layers>/sbom/launch/sbom.legacy.json")
						h.AssertPathDoesNotExist(t, filepath.Join(builder.LayersDir, "sbom", "launch", "sbom.legacy.json"))

						t.Log("it does not save the aggregated legacy build bom to <layers>/sbom/build/sbom.legacy.json")
						h.AssertPathDoesNotExist(t, filepath.Join(builder.LayersDir, "sbom", "build", "sbom.legacy.json"))
					})
				})
			})
		})
	})
}
