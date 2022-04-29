package lifecycle_test

import (
	"reflect"
	"strings"
	"sync"
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
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

//go:generate mockgen -package testmock -destination testmock/resolver.go github.com/buildpacks/lifecycle Resolver

func TestDetector(t *testing.T) {
	spec.Run(t, "Detector", testDetector, spec.Report(report.Terminal{}))
}

func testDetector(t *testing.T, when spec.G, it spec.S) {
	when("#Detect", func() {
		var (
			mockCtrl *gomock.Controller
			detector *lifecycle.Detector
			resolver *testmock.MockResolver
			dirStore *testmock.MockDirStore
		)

		it.Before(func() {
			mockCtrl = gomock.NewController(t)
			dirStore = testmock.NewMockDirStore(mockCtrl)
			var err error
			detector = lifecycle.NewDetector(api.Platform.Latest(), buildpack.DetectConfig{}, dirStore)
			h.AssertNil(t, err)

			// override resolver
			resolver = testmock.NewMockResolver(mockCtrl)
			detector.Resolver = resolver
		})

		it.After(func() {
			mockCtrl.Finish()
		})

		it("should expand order-containing buildpack IDs", func() {
			// This test doesn't use gomock.InOrder() because each call to Detect() happens in a go func.
			// The order that other calls are written in is the order that they happen in.

			bpE1 := testmock.NewMockBuildpack(mockCtrl)
			bpA1 := testmock.NewMockBuildpack(mockCtrl)
			bpF1 := testmock.NewMockBuildpack(mockCtrl)
			bpC1 := testmock.NewMockBuildpack(mockCtrl)
			bpB1 := testmock.NewMockBuildpack(mockCtrl)
			bpG1 := testmock.NewMockBuildpack(mockCtrl)
			bpB2 := testmock.NewMockBuildpack(mockCtrl)
			bpC2 := testmock.NewMockBuildpack(mockCtrl)
			bpD2 := testmock.NewMockBuildpack(mockCtrl)
			bpD1 := testmock.NewMockBuildpack(mockCtrl)

			dirStore.EXPECT().LookupBp("E", "v1").Return(bpE1, nil)
			bpE1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
				API:       "0.2",
				Buildpack: buildpack.Info{ID: "E", Version: "v1"},
				Order: []buildpack.Group{
					{
						Group: []buildpack.GroupBuildpack{
							{ID: "A", Version: "v1"},
							{ID: "F", Version: "v1"},
							{ID: "B", Version: "v1"},
						},
					},
				},
			})

			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
			bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.3", Buildpack: buildpack.Info{ID: "A", Version: "v1"}})
			bpA1.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("F", "v1").Return(bpF1, nil)
			bpF1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
				API:       "0.2",
				Buildpack: buildpack.Info{ID: "F", Version: "v1"},
				Order: []buildpack.Group{
					{Group: []buildpack.GroupBuildpack{
						{ID: "C", Version: "v1"},
					}},
					{Group: []buildpack.GroupBuildpack{
						{ID: "G", Version: "v1", Optional: true},
					}},
					{Group: []buildpack.GroupBuildpack{
						{ID: "D", Version: "v1"},
					}},
				},
			})

			dirStore.EXPECT().LookupBp("C", "v1").Return(bpC1, nil)
			bpC1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "C", Version: "v1"}})
			bpC1.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})
			bpB1.EXPECT().Detect(gomock.Any(), gomock.Any())

			firstGroup := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "C", Version: "v1", API: "0.2"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			firstResolve := resolver.EXPECT().Resolve(
				firstGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupBuildpack{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			)

			dirStore.EXPECT().LookupBp("G", "v1").Return(bpG1, nil)
			bpG1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
				API:       "0.2",
				Buildpack: buildpack.Info{ID: "G", Version: "v1"},
				Order: []buildpack.Group{
					{
						Group: []buildpack.GroupBuildpack{
							{ID: "A", Version: "v2"},
							{ID: "B", Version: "v2"},
						},
					},
					{
						Group: []buildpack.GroupBuildpack{
							{ID: "C", Version: "v2"},
							{ID: "D", Version: "v2"},
						},
					},
				},
			})

			dirStore.EXPECT().LookupBp("B", "v2").Return(bpB2, nil)
			bpB2.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v2"}})
			bpB2.EXPECT().Detect(gomock.Any(), gomock.Any())

			secondGroup := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "B", Version: "v2", API: "0.2"},
			}
			secondResolve := resolver.EXPECT().Resolve(
				secondGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupBuildpack{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			).After(firstResolve)

			dirStore.EXPECT().LookupBp("C", "v2").Return(bpC2, nil)
			bpC2.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "C", Version: "v2"}})
			bpC2.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("D", "v2").Return(bpD2, nil)
			bpD2.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "D", Version: "v2"}})
			bpD2.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})

			thirdGroup := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "C", Version: "v2", API: "0.2"},
				{ID: "D", Version: "v2", API: "0.2"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			thirdResolve := resolver.EXPECT().Resolve(
				thirdGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupBuildpack{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			).After(secondResolve)

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})

			fourthGroup := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			fourthResolve := resolver.EXPECT().Resolve(
				fourthGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupBuildpack{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			).After(thirdResolve)

			dirStore.EXPECT().LookupBp("D", "v1").Return(bpD1, nil)
			bpD1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "D", Version: "v1"}})
			bpD1.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})

			fifthGroup := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "D", Version: "v1", API: "0.2"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			resolver.EXPECT().Resolve(
				fifthGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupBuildpack{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			).After(fourthResolve)

			order := buildpack.Order{
				{Group: []buildpack.GroupBuildpack{{ID: "E", Version: "v1"}}},
			}
			_, _, err := detector.Detect(order, nil)
			if err, ok := err.(*buildpack.Error); !ok || err.Type != buildpack.ErrTypeFailedDetection {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}
		})

		it("should select the first passing group", func() {
			// This test doesn't use gomock.InOrder() because each call to Detect() happens in a go func.
			// The order that other calls are written in is the order that they happen in.

			bpE1 := testmock.NewMockBuildpack(mockCtrl)
			bpA1 := testmock.NewMockBuildpack(mockCtrl)
			bpF1 := testmock.NewMockBuildpack(mockCtrl)
			bpC1 := testmock.NewMockBuildpack(mockCtrl)
			bpB1 := testmock.NewMockBuildpack(mockCtrl)
			bpG1 := testmock.NewMockBuildpack(mockCtrl)
			bpB2 := testmock.NewMockBuildpack(mockCtrl)
			bpC2 := testmock.NewMockBuildpack(mockCtrl)
			bpD2 := testmock.NewMockBuildpack(mockCtrl)

			dirStore.EXPECT().LookupBp("E", "v1").Return(bpE1, nil)
			bpE1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
				API:       "0.2",
				Buildpack: buildpack.Info{ID: "E", Version: "v1"},
				Order: []buildpack.Group{
					{
						Group: []buildpack.GroupBuildpack{
							{ID: "A", Version: "v1"},
							{ID: "F", Version: "v1"},
							{ID: "B", Version: "v1"},
						},
					},
				},
			})

			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
			bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
				API:       "0.3",
				Buildpack: buildpack.Info{ID: "A", Version: "v1", Homepage: "Buildpack A Homepage"},
			})
			bpA1.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("F", "v1").Return(bpF1, nil)
			bpF1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
				API:       "0.2",
				Buildpack: buildpack.Info{ID: "F", Version: "v1"},
				Order: []buildpack.Group{
					{Group: []buildpack.GroupBuildpack{
						{ID: "C", Version: "v1"},
					}},
					{Group: []buildpack.GroupBuildpack{
						{ID: "G", Version: "v1", Optional: true},
					}},
					{Group: []buildpack.GroupBuildpack{
						{ID: "D", Version: "v1"},
					}},
				},
			})

			dirStore.EXPECT().LookupBp("C", "v1").Return(bpC1, nil)
			bpC1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "C", Version: "v1"}})
			bpC1.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})
			bpB1.EXPECT().Detect(gomock.Any(), gomock.Any())

			firstGroup := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"},
				{ID: "C", Version: "v1", API: "0.2"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			firstResolve := resolver.EXPECT().Resolve(
				firstGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupBuildpack{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			)

			dirStore.EXPECT().LookupBp("G", "v1").Return(bpG1, nil)
			bpG1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
				API:       "0.2",
				Buildpack: buildpack.Info{ID: "G", Version: "v1"},
				Order: []buildpack.Group{
					{
						Group: []buildpack.GroupBuildpack{
							{ID: "A", Version: "v2"},
							{ID: "B", Version: "v2"},
						},
					},
					{
						Group: []buildpack.GroupBuildpack{
							{ID: "C", Version: "v2"},
							{ID: "D", Version: "v2"},
						},
					},
				},
			})

			dirStore.EXPECT().LookupBp("B", "v2").Return(bpB2, nil)
			bpB2.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v2"}})
			bpB2.EXPECT().Detect(gomock.Any(), gomock.Any())

			secondGroup := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"},
				{ID: "B", Version: "v2", API: "0.2"},
			}
			secondResolve := resolver.EXPECT().Resolve(
				secondGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupBuildpack{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			).After(firstResolve)

			dirStore.EXPECT().LookupBp("C", "v2").Return(bpC2, nil)
			bpC2.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "C", Version: "v2"}})
			bpC2.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("D", "v2").Return(bpD2, nil)
			bpD2.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "D", Version: "v2"}})
			bpD2.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})

			thirdGroup := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"},
				{ID: "C", Version: "v2", API: "0.2"},
				{ID: "D", Version: "v2", API: "0.2"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			thirdResolve := resolver.EXPECT().Resolve(
				thirdGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupBuildpack{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			).After(secondResolve)

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})

			fourthGroup := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			resolver.EXPECT().Resolve(
				fourthGroup,
				detector.Runs,
			).Return(
				fourthGroup,
				[]platform.BuildPlanEntry{},
				nil,
			).After(thirdResolve)

			order := buildpack.Order{
				{Group: []buildpack.GroupBuildpack{{ID: "E", Version: "v1"}}},
			}
			group, plan, err := detector.Detect(order, nil)
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(group, buildpack.Group{
				Group: []buildpack.GroupBuildpack{
					{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"},
					{ID: "B", Version: "v1", API: "0.2"},
				},
			}); s != "" {
				t.Fatalf("Unexpected group:\n%s\n", s)
			}

			if !hasEntries(plan.Entries, []platform.BuildPlanEntry(nil)) {
				t.Fatalf("Unexpected entries:\n%+v\n", plan.Entries)
			}
		})

		it("should convert top level versions to metadata versions", func() {
			bpA1 := testmock.NewMockBuildpack(mockCtrl)
			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
			bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.3", Buildpack: buildpack.Info{ID: "A", Version: "v1"}})
			bpA1.EXPECT().Detect(gomock.Any(), gomock.Any())

			bpB1 := testmock.NewMockBuildpack(mockCtrl)
			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})
			bpB1.EXPECT().Detect(gomock.Any(), gomock.Any())

			group := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			resolver.EXPECT().Resolve(group, detector.Runs).Return(group, []platform.BuildPlanEntry{
				{
					Providers: []buildpack.GroupBuildpack{
						{ID: "A", Version: "v1"},
					},
					Requires: []buildpack.Require{
						{
							Name:    "dep1",
							Version: "some-version",
						},
					},
				},
				{
					Providers: []buildpack.GroupBuildpack{
						{ID: "B", Version: "v1"},
					},
					Requires: []buildpack.Require{
						{
							Name:     "dep2",
							Version:  "some-already-exists-version",
							Metadata: map[string]interface{}{"version": "some-already-exists-version"},
						},
					},
				},
			}, nil)

			found, plan, err := detector.Detect(buildpack.Order{{Group: group}}, nil)
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(found, buildpack.Group{Group: group}); s != "" {
				t.Fatalf("Unexpected group:\n%s\n", s)
			}

			if !hasEntries(plan.Entries, []platform.BuildPlanEntry{
				{
					Providers: []buildpack.GroupBuildpack{
						{ID: "A", Version: "v1"},
					},
					Requires: []buildpack.Require{
						{Name: "dep1", Metadata: map[string]interface{}{"version": "some-version"}},
					},
				},
				{
					Providers: []buildpack.GroupBuildpack{
						{ID: "B", Version: "v1"},
					},
					Requires: []buildpack.Require{
						{Name: "dep2", Metadata: map[string]interface{}{"version": "some-already-exists-version"}},
					},
				},
			}) {
				t.Fatalf("Unexpected entries:\n%+v\n", plan.Entries)
			}
		})

		it("should update detect runs for each buildpack", func() {
			bpA1 := testmock.NewMockBuildpack(mockCtrl)
			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
			bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.3", Buildpack: buildpack.Info{ID: "A", Version: "v1"}})
			bpA1.EXPECT().Detect(gomock.Any(), gomock.Any()).Return(buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Requires: []buildpack.Require{{Name: "some-dep"}},
						Provides: []buildpack.Provide{{Name: "some-dep"}},
					},
					Or: []buildpack.PlanSections{
						{
							Requires: []buildpack.Require{{Name: "some-other-dep"}},
							Provides: []buildpack.Provide{{Name: "some-other-dep"}},
						},
					},
				},
				Output: []byte("detect out: A@v1\ndetect err: A@v1"),
				Code:   0,
			})

			bpB1 := testmock.NewMockBuildpack(mockCtrl)
			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})
			bpBerror := errors.New("some-error")
			bpB1.EXPECT().Detect(gomock.Any(), gomock.Any()).Return(buildpack.DetectRun{
				Output: []byte("detect out: B@v1\ndetect err: B@v1"),
				Code:   100,
				Err:    bpBerror,
			})

			group := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			resolver.EXPECT().Resolve(group, detector.Runs).Return(group, []platform.BuildPlanEntry{}, nil)

			_, _, err := detector.Detect(buildpack.Order{{Group: group}}, nil)
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			bpARun, ok := detector.Runs.Load("A@v1")
			if !ok {
				t.Fatalf("missing detection of '%s'", "A@v1")
			}
			if s := cmp.Diff(bpARun, buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Requires: []buildpack.Require{{Name: "some-dep"}},
						Provides: []buildpack.Provide{{Name: "some-dep"}},
					},
					Or: []buildpack.PlanSections{
						{
							Requires: []buildpack.Require{{Name: "some-other-dep"}},
							Provides: []buildpack.Provide{{Name: "some-other-dep"}},
						},
					},
				},
				Output: []byte("detect out: A@v1\ndetect err: A@v1"),
				Code:   0,
				Err:    nil,
			}); s != "" {
				t.Fatalf("Unexpected detect run:\n%s\n", s)
			}

			bpBRun, ok := detector.Runs.Load("B@v1")
			if !ok {
				t.Fatalf("missing detection of '%s'", "B@v1")
			}
			if s := cmp.Diff(bpBRun, buildpack.DetectRun{
				Output: []byte("detect out: B@v1\ndetect err: B@v1"),
				Code:   100,
				Err:    bpBerror,
			}, cmp.Comparer(errors.Is)); s != "" {
				t.Fatalf("Unexpected detect run:\n%s\n", s)
			}
		})

		when("resolve errors", func() {
			when("with buildpack error", func() {
				it("returns a buildpack error", func() {
					bpA1 := testmock.NewMockBuildpack(mockCtrl)
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
					bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.3", Buildpack: buildpack.Info{ID: "A", Version: "v1"}})
					bpA1.EXPECT().Detect(gomock.Any(), gomock.Any())

					group := []buildpack.GroupBuildpack{
						{ID: "A", Version: "v1", API: "0.3"},
					}
					resolver.EXPECT().Resolve(group, detector.Runs).Return(
						[]buildpack.GroupBuildpack{},
						[]platform.BuildPlanEntry{},
						lifecycle.ErrBuildpack,
					)

					_, _, err := detector.Detect(buildpack.Order{{Group: group}}, nil)
					if err, ok := err.(*buildpack.Error); !ok || err.Type != buildpack.ErrTypeBuildpack {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
				})
			})

			when("with detect error", func() {
				it("returns a detect error", func() {
					bpA1 := testmock.NewMockBuildpack(mockCtrl)
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
					bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.3", Buildpack: buildpack.Info{ID: "A", Version: "v1"}})
					bpA1.EXPECT().Detect(gomock.Any(), gomock.Any())

					group := []buildpack.GroupBuildpack{
						{ID: "A", Version: "v1", API: "0.3"},
					}
					resolver.EXPECT().Resolve(group, detector.Runs).Return(
						[]buildpack.GroupBuildpack{},
						[]platform.BuildPlanEntry{},
						lifecycle.ErrFailedDetection,
					)

					_, _, err := detector.Detect(buildpack.Order{{Group: group}}, nil)
					if err, ok := err.(*buildpack.Error); !ok || err.Type != buildpack.ErrTypeFailedDetection {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
				})
			})
		})

		when("provided an order for extensions", func() {
			it("prepends the order to each buildpack group and selects the first passing group", func() {
				bpA1 := testmock.NewMockBuildpack(mockCtrl)
				bpB1 := testmock.NewMockBuildpack(mockCtrl)
				extC1 := testmock.NewMockBuildpack(mockCtrl)
				extD1 := testmock.NewMockBuildpack(mockCtrl)

				dirStore.EXPECT().LookupExt("C", "v1").Return(extC1, nil)
				extC1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.9",
					Extension: buildpack.Info{ID: "C", Version: "v1"},
				})
				extC1.EXPECT().Detect(gomock.Any(), gomock.Any())

				dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
				bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.8",
					Buildpack: buildpack.Info{ID: "A", Version: "v1"},
				})
				bpA1.EXPECT().Detect(gomock.Any(), gomock.Any())

				firstGroup := []buildpack.GroupBuildpack{
					{ID: "C", Version: "v1", API: "0.9"},
					{ID: "A", Version: "v1", API: "0.8"},
				}
				firstResolve := resolver.EXPECT().Resolve(
					firstGroup,
					detector.Runs,
				).Return(
					[]buildpack.GroupBuildpack{},
					[]platform.BuildPlanEntry{},
					lifecycle.ErrFailedDetection,
				)

				dirStore.EXPECT().LookupExt("D", "v1").Return(extD1, nil)
				extD1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.9",
					Extension: buildpack.Info{ID: "D", Version: "v1"},
				})
				extD1.EXPECT().Detect(gomock.Any(), gomock.Any())

				dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
				bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.8",
					Buildpack: buildpack.Info{ID: "A", Version: "v1"},
				})

				secondGroup := []buildpack.GroupBuildpack{
					{ID: "D", Version: "v1", API: "0.9"},
					{ID: "A", Version: "v1", API: "0.8"},
				}
				secondResolve := resolver.EXPECT().Resolve(
					secondGroup,
					detector.Runs,
				).Return(
					[]buildpack.GroupBuildpack{},
					[]platform.BuildPlanEntry{},
					lifecycle.ErrFailedDetection,
				).After(firstResolve)

				dirStore.EXPECT().LookupExt("C", "v1").Return(extC1, nil)
				extC1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.9",
					Extension: buildpack.Info{ID: "C", Version: "v1"},
				})

				dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
				bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.8",
					Buildpack: buildpack.Info{ID: "B", Version: "v1"},
				})
				bpB1.EXPECT().Detect(gomock.Any(), gomock.Any())

				thirdGroup := []buildpack.GroupBuildpack{
					{ID: "C", Version: "v1", API: "0.9"},
					{ID: "B", Version: "v1", API: "0.8"},
				}
				thirdResolve := resolver.EXPECT().Resolve(
					thirdGroup,
					detector.Runs,
				).Return(
					[]buildpack.GroupBuildpack{},
					[]platform.BuildPlanEntry{},
					lifecycle.ErrFailedDetection,
				).After(secondResolve)

				dirStore.EXPECT().LookupExt("D", "v1").Return(extD1, nil)
				extD1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.9",
					Extension: buildpack.Info{ID: "D", Version: "v1"},
				})

				dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
				bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.8",
					Buildpack: buildpack.Info{ID: "B", Version: "v1"},
				})

				fourthGroup := []buildpack.GroupBuildpack{
					{ID: "D", Version: "v1", API: "0.9"},
					{ID: "B", Version: "v1", API: "0.8"},
				}
				resolver.EXPECT().Resolve(
					fourthGroup,
					detector.Runs,
				).Return(
					fourthGroup,
					[]platform.BuildPlanEntry{},
					nil,
				).After(thirdResolve)

				order := buildpack.Order{
					{Group: []buildpack.GroupBuildpack{{ID: "A", Version: "v1"}}},
					{Group: []buildpack.GroupBuildpack{{ID: "B", Version: "v1"}}},
				}
				orderExt := buildpack.Order{
					{Group: []buildpack.GroupBuildpack{{ID: "C", Version: "v1"}}},
					{Group: []buildpack.GroupBuildpack{{ID: "D", Version: "v1"}}},
				}

				_, _, err := detector.Detect(order, orderExt)
				h.AssertNil(t, err)
			})
		})
	})

	when("#Resolve", func() {
		var (
			logHandler *memory.Handler
			resolver   *lifecycle.DefaultResolver
		)

		it.Before(func() {
			logHandler = memory.New()
			resolver = &lifecycle.DefaultResolver{
				Logger: &log.Logger{Handler: logHandler},
			}
		})

		it("should fail if the group is empty", func() {
			_, _, err := resolver.Resolve([]buildpack.GroupBuildpack{}, &sync.Map{})
			if err != lifecycle.ErrFailedDetection {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(h.AllLogs(logHandler),
				"======== Results ========\n"+
					"Resolving plan... (try #1)\n"+
					"fail: no viable buildpacks in group\n",
			); s != "" {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should fail if the group has no viable buildpacks, even if no required buildpacks fail", func() {
			group := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", Optional: true},
				{ID: "B", Version: "v1", Optional: true},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("A@v1", buildpack.DetectRun{
				Code: 100,
			})
			detectRuns.Store("B@v1", buildpack.DetectRun{
				Code: 100,
			})

			_, _, err := resolver.Resolve(group, detectRuns)
			if err != lifecycle.ErrFailedDetection {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := h.AllLogs(logHandler); !strings.HasSuffix(s,
				"======== Results ========\n"+
					"skip: A@v1\n"+
					"skip: B@v1\n"+
					"Resolving plan... (try #1)\n"+
					"fail: no viable buildpacks in group\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should fail with specific error if any bp detect fails in an unexpected way", func() {
			group := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", Optional: false},
				{ID: "B", Version: "v1", Optional: false},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("A@v1", buildpack.DetectRun{
				Code: 0,
			})
			detectRuns.Store("B@v1", buildpack.DetectRun{
				Code: 127,
			})

			_, _, err := resolver.Resolve(group, detectRuns)
			if err != lifecycle.ErrBuildpack {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := h.AllLogs(logHandler); !strings.HasSuffix(s,
				"======== Results ========\n"+
					"pass: A@v1\n"+
					"err:  B@v1 (127)\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should not output detect pass and fail as info level", func() {
			group := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", Optional: false},
				{ID: "B", Version: "v1", Optional: false},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("A@v1", buildpack.DetectRun{
				Code: 0,
			})
			detectRuns.Store("B@v1", buildpack.DetectRun{
				Code: 100,
			})

			resolver.Logger = &log.Logger{Handler: logHandler, Level: log.InfoLevel}

			_, _, err := resolver.Resolve(group, detectRuns)
			if err != lifecycle.ErrFailedDetection {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := h.AllLogs(logHandler); s != "" {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should output detect errors as info level", func() {
			group := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", Optional: false},
				{ID: "B", Version: "v1", Optional: false},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("A@v1", buildpack.DetectRun{
				Code: 0,
			})
			detectRuns.Store("B@v1", buildpack.DetectRun{
				Output: []byte("detect out: B@v1\ndetect err: B@v1"),
				Code:   127,
			})

			resolver.Logger = &log.Logger{Handler: logHandler, Level: log.InfoLevel}

			_, _, err := resolver.Resolve(group, detectRuns)
			if err != lifecycle.ErrBuildpack {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := h.AllLogs(logHandler); !strings.HasSuffix(s,
				"======== Output: B@v1 ========\n"+
					"detect out: B@v1\n"+
					"detect err: B@v1\n"+
					"err:  B@v1 (127)\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should return a build plan with matched dependencies", func() {
			group := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"},
				{ID: "C", Version: "v2", API: "0.2"},
				{ID: "D", Version: "v2", API: "0.2"},
				{ID: "B", Version: "v1", API: "0.2"},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("A@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep1"},
							{Name: "dep2"},
						},
						Requires: []buildpack.Require{
							{Name: "dep2"},
						},
					},
				},
			})
			detectRuns.Store("B@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Requires: []buildpack.Require{
							{Name: "dep1"},
							{Name: "dep2"},
						},
					},
				},
			})
			detectRuns.Store("C@v2", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep1"},
							{Name: "dep2"},
						},
					},
				},
			})
			detectRuns.Store("D@v2", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep2"},
						},
						Requires: []buildpack.Require{
							{Name: "dep1"},
							{Name: "dep2"},
						},
					},
				},
			})

			found, entries, err := resolver.Resolve(group, detectRuns)
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(found, group); s != "" {
				t.Fatalf("Unexpected group:\n%s\n", s)
			}

			if !hasEntries(entries, []platform.BuildPlanEntry{
				{
					Providers: []buildpack.GroupBuildpack{
						{ID: "A", Version: "v1"},
						{ID: "C", Version: "v2"},
					},
					Requires: []buildpack.Require{{Name: "dep1"}, {Name: "dep1"}},
				},
				{
					Providers: []buildpack.GroupBuildpack{
						{ID: "A", Version: "v1"},
						{ID: "C", Version: "v2"},
						{ID: "D", Version: "v2"},
					},
					Requires: []buildpack.Require{{Name: "dep2"}, {Name: "dep2"}, {Name: "dep2"}},
				},
			}) {
				t.Fatalf("Unexpected entries:\n%+v\n", entries)
			}

			if s := h.AllLogs(logHandler); !strings.HasSuffix(s,
				"======== Results ========\n"+
					"pass: A@v1\n"+
					"pass: C@v2\n"+
					"pass: D@v2\n"+
					"pass: B@v1\n"+
					"Resolving plan... (try #1)\n"+
					"A v1\n"+
					"C v2\n"+
					"D v2\n"+
					"B v1\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should fail if all requires are not provided first", func() {
			group := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", Optional: true},
				{ID: "B", Version: "v1"},
				{ID: "C", Version: "v1"},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("A@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep1"},
						},
					},
				},
				Code: 100,
			})
			detectRuns.Store("B@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Requires: []buildpack.Require{
							{Name: "dep1"},
						},
					},
				},
			})
			detectRuns.Store("C@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep1"},
						},
						Requires: []buildpack.Require{
							{Name: "dep1"},
						},
					},
				},
			})

			_, _, err := resolver.Resolve(group, detectRuns)
			if err != lifecycle.ErrFailedDetection {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := h.AllLogs(logHandler); !strings.HasSuffix(s,
				"======== Results ========\n"+
					"skip: A@v1\n"+
					"pass: B@v1\n"+
					"pass: C@v1\n"+
					"Resolving plan... (try #1)\n"+
					"fail: B@v1 requires dep1\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should fail if all provides are not required after", func() {
			group := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1"},
				{ID: "B", Version: "v1"},
				{ID: "C", Version: "v1", Optional: true},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("A@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep1"},
						},
						Requires: []buildpack.Require{
							{Name: "dep1"},
						},
					},
				},
			})
			detectRuns.Store("B@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep1"},
						},
					},
				},
			})
			detectRuns.Store("C@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Requires: []buildpack.Require{
							{Name: "dep1"},
						},
					},
				},
				Code: 100,
			})

			_, _, err := resolver.Resolve(group, detectRuns)
			if err != lifecycle.ErrFailedDetection {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := h.AllLogs(logHandler); !strings.HasSuffix(s,
				"======== Results ========\n"+
					"pass: A@v1\n"+
					"pass: B@v1\n"+
					"skip: C@v1\n"+
					"Resolving plan... (try #1)\n"+
					"fail: B@v1 provides unused dep1\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should succeed if unmet provides/requires are optional", func() {
			group := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", Optional: true},
				{ID: "B", Version: "v1", API: "0.2"},
				{ID: "C", Version: "v1", Optional: true},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("A@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Requires: []buildpack.Require{
							{Name: "dep-missing"},
						},
					},
				},
			})
			detectRuns.Store("B@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep-present"},
						},
						Requires: []buildpack.Require{
							{Name: "dep-present"},
						},
					},
				},
			})
			detectRuns.Store("C@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep-missing"},
						},
					},
				},
			})

			found, entries, err := resolver.Resolve(group, detectRuns)
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(found, []buildpack.GroupBuildpack{
				{ID: "B", Version: "v1", API: "0.2"},
			}); s != "" {
				t.Fatalf("Unexpected group:\n%s\n", s)
			}

			if !hasEntries(entries, []platform.BuildPlanEntry{
				{
					Providers: []buildpack.GroupBuildpack{{ID: "B", Version: "v1"}},
					Requires:  []buildpack.Require{{Name: "dep-present"}},
				},
			}) {
				t.Fatalf("Unexpected entries:\n%+v\n", entries)
			}

			if s := h.AllLogs(logHandler); !strings.HasSuffix(s,
				"======== Results ========\n"+
					"pass: A@v1\n"+
					"pass: B@v1\n"+
					"pass: C@v1\n"+
					"Resolving plan... (try #1)\n"+
					"skip: A@v1 requires dep-missing\n"+
					"skip: C@v1 provides unused dep-missing\n"+
					"1 of 3 buildpacks participating\n"+
					"B v1\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("should fallback to alternate build plans", func() {
			group := []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", Optional: true, API: "0.3", Homepage: "Buildpack A Homepage"},
				{ID: "B", Version: "v1", Optional: true, API: "0.2"},
				{ID: "C", Version: "v1", API: "0.2"},
				{ID: "D", Version: "v1", Optional: true},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("A@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep2-missing"},
						},
					},
					Or: []buildpack.PlanSections{
						{
							Provides: []buildpack.Provide{
								{Name: "dep1-present"},
							},
						},
					},
				},
			})
			detectRuns.Store("B@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Requires: []buildpack.Require{
							{Name: "dep3-missing"},
						},
					},
					Or: []buildpack.PlanSections{
						{
							Requires: []buildpack.Require{
								{Name: "dep1-present"},
							},
						},
					},
				},
			})
			detectRuns.Store("C@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep5-missing"},
						},
						Requires: []buildpack.Require{
							{Name: "dep4-missing"},
						},
					},
					Or: []buildpack.PlanSections{
						{
							Provides: []buildpack.Provide{
								{Name: "dep6-present"},
							},
							Requires: []buildpack.Require{
								{Name: "dep6-present"},
							},
						},
					},
				},
			})
			detectRuns.Store("D@v1", buildpack.DetectRun{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep8-missing"},
						},
						Requires: []buildpack.Require{
							{Name: "dep7-missing"},
						},
					},
					Or: []buildpack.PlanSections{
						{
							Provides: []buildpack.Provide{
								{Name: "dep10-missing"},
							},
							Requires: []buildpack.Require{
								{Name: "dep9-missing"},
							},
						},
					},
				},
			})

			found, entries, err := resolver.Resolve(group, detectRuns)
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(found, []buildpack.GroupBuildpack{
				{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"},
				{ID: "B", Version: "v1", API: "0.2"},
				{ID: "C", Version: "v1", API: "0.2"},
			}); s != "" {
				t.Fatalf("Unexpected group:\n%s\n", s)
			}

			if !hasEntries(entries, []platform.BuildPlanEntry{
				{
					Providers: []buildpack.GroupBuildpack{{ID: "A", Version: "v1"}},
					Requires:  []buildpack.Require{{Name: "dep1-present"}},
				},
				{
					Providers: []buildpack.GroupBuildpack{{ID: "C", Version: "v1"}},
					Requires:  []buildpack.Require{{Name: "dep6-present"}},
				},
			}) {
				t.Fatalf("Unexpected entries:\n%+v\n", entries)
			}

			if s := h.AllLogs(logHandler); !strings.HasSuffix(s,
				"Resolving plan... (try #16)\n"+
					"skip: D@v1 requires dep9-missing\n"+
					"skip: D@v1 provides unused dep10-missing\n"+
					"3 of 4 buildpacks participating\n"+
					"A v1\n"+
					"B v1\n"+
					"C v1\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})
	})
}

func hasEntry(l []platform.BuildPlanEntry, entry platform.BuildPlanEntry) bool {
	for _, e := range l {
		if reflect.DeepEqual(e, entry) {
			return true
		}
	}
	return false
}

func hasEntries(a, b []platform.BuildPlanEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for _, e := range a {
		if !hasEntry(b, e) {
			return false
		}
	}
	return true
}
