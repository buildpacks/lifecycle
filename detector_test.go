package lifecycle_test

import (
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	apexlog "github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

func TestDetector(t *testing.T) {
	spec.Run(t, "Detector", testDetector, spec.Report(report.Terminal{}))
}

func testDetector(t *testing.T, when spec.G, it spec.S) {
	var (
		mockController *gomock.Controller

		apiVerifier   *testmock.MockBuildpackAPIVerifier
		configHandler *testmock.MockConfigHandler
		dirStore      *testmock.MockDirStore
		logger        log.LoggerHandlerWithLevel

		detectorFactory *lifecycle.DetectorFactory
	)

	it.Before(func() {
		mockController = gomock.NewController(t)

		apiVerifier = testmock.NewMockBuildpackAPIVerifier(mockController)
		configHandler = testmock.NewMockConfigHandler(mockController)
		dirStore = testmock.NewMockDirStore(mockController)
		logger = log.NewDefaultLogger(io.Discard)

		detectorFactory = lifecycle.NewDetectorFactory(
			api.Platform.Latest(),
			apiVerifier,
			configHandler,
			dirStore,
		)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#NewDetector", func() {
		it("configures the detector", func() {
			order := buildpack.Order{
				buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
			}
			configHandler.EXPECT().ReadOrder("some-order-path").Return(order, nil, nil)

			t.Log("verifies buildpack apis")
			bpA1 := &buildpack.BpDescriptor{WithAPI: "0.2"}
			dirStore.EXPECT().Lookup(buildpack.KindBuildpack, "A", "v1").Return(bpA1, nil)
			apiVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindBuildpack, "A@v1", "0.2", logger)
			amd := platform.AnalyzedMetadata{}

			detector, err := detectorFactory.NewDetector(amd, "some-app-dir", "some-build-config-dir", "some-order-path", "some-platform-dir", logger)
			h.AssertNil(t, err)

			h.AssertEq(t, detector.AppDir, "some-app-dir")
			h.AssertEq(t, detector.BuildConfigDir, "some-build-config-dir")
			h.AssertNotNil(t, detector.DirStore)
			h.AssertEq(t, detector.HasExtensions, false)
			h.AssertEq(t, detector.Order, order)
			h.AssertEq(t, detector.PlatformDir, "some-platform-dir")
			_, ok := detector.Resolver.(*lifecycle.DefaultDetectResolver)
			h.AssertEq(t, ok, true)
			h.AssertNotNil(t, detector.Runs)
		})

		when("there are extensions", func() {
			it("prepends the extensions order to the buildpacks order", func() {
				orderBp := buildpack.Order{
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "B", Version: "v1"}}},
				}
				orderExt := buildpack.Order{
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "C", Version: "v1", Extension: true}}},
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "D", Version: "v1", Extension: true}}},
				}
				expectedOrder := buildpack.Order{
					buildpack.Group{
						Group: []buildpack.GroupElement{
							{OrderExtensions: buildpack.Order{
								buildpack.Group{Group: []buildpack.GroupElement{{ID: "C", Version: "v1", Extension: true, Optional: true}}},
								buildpack.Group{Group: []buildpack.GroupElement{{ID: "D", Version: "v1", Extension: true, Optional: true}}},
							}},
							{ID: "A", Version: "v1"},
						},
					},
					buildpack.Group{
						Group: []buildpack.GroupElement{
							{OrderExtensions: buildpack.Order{
								buildpack.Group{Group: []buildpack.GroupElement{{ID: "C", Version: "v1", Extension: true, Optional: true}}},
								buildpack.Group{Group: []buildpack.GroupElement{{ID: "D", Version: "v1", Extension: true, Optional: true}}},
							}},
							{ID: "B", Version: "v1"},
						},
					},
				}
				configHandler.EXPECT().ReadOrder("some-order-path").Return(orderBp, orderExt, nil)

				t.Log("verifies buildpack apis")
				bpA1 := &buildpack.BpDescriptor{WithAPI: "0.2"}
				bpB1 := &buildpack.BpDescriptor{WithAPI: "0.2"}
				extC1 := &buildpack.BpDescriptor{WithAPI: "0.10"}
				extD1 := &buildpack.BpDescriptor{WithAPI: "0.10"}
				dirStore.EXPECT().Lookup(buildpack.KindBuildpack, "A", "v1").Return(bpA1, nil)
				apiVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindBuildpack, "A@v1", "0.2", logger)
				dirStore.EXPECT().Lookup(buildpack.KindBuildpack, "B", "v1").Return(bpB1, nil)
				apiVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindBuildpack, "B@v1", "0.2", logger)
				dirStore.EXPECT().Lookup(buildpack.KindExtension, "C", "v1").Return(extC1, nil)
				apiVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindExtension, "C@v1", "0.10", logger)
				dirStore.EXPECT().Lookup(buildpack.KindExtension, "D", "v1").Return(extD1, nil)
				apiVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindExtension, "D@v1", "0.10", logger)

				amd := platform.AnalyzedMetadata{}
				detector, err := detectorFactory.NewDetector(amd, "some-app-dir", "some-build-config-dir", "some-order-path", "some-platform-dir", logger)
				h.AssertNil(t, err)

				h.AssertEq(t, detector.AppDir, "some-app-dir")
				h.AssertNotNil(t, detector.DirStore)
				h.AssertEq(t, detector.HasExtensions, true)
				h.AssertEq(t, detector.Order, expectedOrder)
				h.AssertEq(t, detector.PlatformDir, "some-platform-dir")
				_, ok := detector.Resolver.(*lifecycle.DefaultDetectResolver)
				h.AssertEq(t, ok, true)
				h.AssertNotNil(t, detector.Runs)
			})
		})
	})

	when(".Detect", func() {
		var (
			detector *lifecycle.Detector
			executor *testmock.MockDetectExecutor
			resolver *testmock.MockDetectResolver
		)

		it.Before(func() {
			configHandler.EXPECT().ReadOrder(gomock.Any()).Return(buildpack.Order{}, buildpack.Order{}, nil)
			amd := platform.AnalyzedMetadata{}
			var err error
			detector, err = detectorFactory.NewDetector(
				amd,
				"some-app-dir",
				"some-build-config-dir",
				"some-order-path",
				"some-platform-dir",
				logger,
			)
			h.AssertNil(t, err)
			// override factory-provided services
			executor = testmock.NewMockDetectExecutor(mockController)
			resolver = testmock.NewMockDetectResolver(mockController)
			detector.Executor = executor
			detector.Resolver = resolver
		})

		it("provides detect inputs to each group element", func() {
			bpA1 := &buildpack.BpDescriptor{
				WithAPI:   "0.3",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}},
			}
			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil).AnyTimes()
			executor.EXPECT().Detect(bpA1, gomock.Any(), gomock.Any()).Do(
				func(_ buildpack.Descriptor, inputs buildpack.DetectInputs, _ log.Logger) buildpack.DetectOutputs {
					h.AssertEq(t, inputs.AppDir, detector.AppDir)
					h.AssertEq(t, inputs.BuildConfigDir, detector.BuildConfigDir)
					h.AssertEq(t, inputs.PlatformDir, detector.PlatformDir)
					return buildpack.DetectOutputs{}
				})

			group := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3", Optional: true},
			}
			resolver.EXPECT().Resolve(group, detector.Runs)

			detector.Order = buildpack.Order{{Group: group}}
			_, _, _ = detector.Detect()
		})

		it("expands order-containing buildpack IDs", func() {
			// This test doesn't use gomock.InOrder() because each call to Detect() happens in a go func.
			// The order that other calls are written in is the order that they happen in.

			bpE1 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "E", Version: "v1"}},
				Order: []buildpack.Group{
					{
						Group: []buildpack.GroupElement{
							{ID: "A", Version: "v1"},
							{ID: "F", Version: "v1"},
							{ID: "B", Version: "v1"},
						},
					},
				},
			}
			dirStore.EXPECT().LookupBp("E", "v1").Return(bpE1, nil).AnyTimes()
			bpA1 := &buildpack.BpDescriptor{
				WithAPI:   "0.3",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}},
			}
			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil).AnyTimes()
			bpF1 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "F", Version: "v1"}},
				Order: []buildpack.Group{
					{Group: []buildpack.GroupElement{
						{ID: "C", Version: "v1"},
					}},
					{Group: []buildpack.GroupElement{
						{ID: "G", Version: "v1", Optional: true},
					}},
					{Group: []buildpack.GroupElement{
						{ID: "D", Version: "v1"},
					}},
				},
			}
			dirStore.EXPECT().LookupBp("F", "v1").Return(bpF1, nil).AnyTimes()
			bpC1 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "C", Version: "v1"}},
			}
			dirStore.EXPECT().LookupBp("C", "v1").Return(bpC1, nil).AnyTimes()
			bpB1 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}},
			}
			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil).AnyTimes()
			bpG1 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "G", Version: "v1"}},
				Order: []buildpack.Group{
					{
						Group: []buildpack.GroupElement{
							{ID: "A", Version: "v2"},
							{ID: "B", Version: "v2"},
						},
					},
					{
						Group: []buildpack.GroupElement{
							{ID: "C", Version: "v2"},
							{ID: "D", Version: "v2"},
						},
					},
				},
			}
			dirStore.EXPECT().LookupBp("G", "v1").Return(bpG1, nil).AnyTimes()
			bpB2 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v2"}},
			}
			dirStore.EXPECT().LookupBp("B", "v2").Return(bpB2, nil).AnyTimes()
			bpC2 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "C", Version: "v2"}},
			}
			dirStore.EXPECT().LookupBp("C", "v2").Return(bpC2, nil).AnyTimes()
			bpD2 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "D", Version: "v2"}},
			}
			dirStore.EXPECT().LookupBp("D", "v2").Return(bpD2, nil).AnyTimes()
			bpD1 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "D", Version: "v1"}},
			}
			dirStore.EXPECT().LookupBp("D", "v1").Return(bpD1, nil).AnyTimes()

			executor.EXPECT().Detect(bpA1, gomock.Any(), gomock.Any())
			executor.EXPECT().Detect(bpC1, gomock.Any(), gomock.Any())
			executor.EXPECT().Detect(bpB1, gomock.Any(), gomock.Any())

			firstGroup := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "C", Version: "v1", API: "0.2"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			firstResolve := resolver.EXPECT().Resolve(
				firstGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupElement{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			)

			// bpA1 already done
			executor.EXPECT().Detect(bpB2, gomock.Any(), gomock.Any())

			secondGroup := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "B", Version: "v2", API: "0.2"},
			}
			secondResolve := resolver.EXPECT().Resolve(
				secondGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupElement{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			).After(firstResolve)

			// bpA1 already done
			executor.EXPECT().Detect(bpC2, gomock.Any(), gomock.Any())
			executor.EXPECT().Detect(bpD2, gomock.Any(), gomock.Any())
			// bpB1 already done

			thirdGroup := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "C", Version: "v2", API: "0.2"},
				{ID: "D", Version: "v2", API: "0.2"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			thirdResolve := resolver.EXPECT().Resolve(
				thirdGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupElement{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			).After(secondResolve)

			// bpA1 already done
			// bpB1 already done

			fourthGroup := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			fourthResolve := resolver.EXPECT().Resolve(
				fourthGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupElement{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			).After(thirdResolve)

			// bpA1 already done
			executor.EXPECT().Detect(bpD1, gomock.Any(), gomock.Any())
			// bpB1 already done

			fifthGroup := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "D", Version: "v1", API: "0.2"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			resolver.EXPECT().Resolve(
				fifthGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupElement{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			).After(fourthResolve)

			order := buildpack.Order{
				{Group: []buildpack.GroupElement{{ID: "E", Version: "v1"}}},
			}
			detector.Order = order
			_, _, err := detector.Detect()
			if err, ok := err.(*buildpack.Error); !ok || err.Type != buildpack.ErrTypeFailedDetection {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}
		})

		it("selects the first passing group", func() {
			// This test doesn't use gomock.InOrder() because each call to Detect() happens in a go func.
			// The order that other calls are written in is the order that they happen in.

			bpE1 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "E", Version: "v1"}},
				Order: []buildpack.Group{
					{
						Group: []buildpack.GroupElement{
							{ID: "A", Version: "v1"},
							{ID: "F", Version: "v1"},
							{ID: "B", Version: "v1"},
						},
					},
				},
			}
			dirStore.EXPECT().LookupBp("E", "v1").Return(bpE1, nil).AnyTimes()
			bpA1 := &buildpack.BpDescriptor{
				WithAPI:   "0.3",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1", Homepage: "Buildpack A Homepage"}}, // homepage added intentionally
			}
			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil).AnyTimes()
			bpF1 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "F", Version: "v1"}},
				Order: []buildpack.Group{
					{Group: []buildpack.GroupElement{
						{ID: "C", Version: "v1"},
					}},
					{Group: []buildpack.GroupElement{
						{ID: "G", Version: "v1", Optional: true},
					}},
					{Group: []buildpack.GroupElement{
						{ID: "D", Version: "v1"},
					}},
				},
			}
			dirStore.EXPECT().LookupBp("F", "v1").Return(bpF1, nil).AnyTimes()
			bpC1 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "C", Version: "v1"}},
			}
			dirStore.EXPECT().LookupBp("C", "v1").Return(bpC1, nil).AnyTimes()
			bpB1 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}},
			}
			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil).AnyTimes()
			bpG1 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "G", Version: "v1"}},
				Order: []buildpack.Group{
					{
						Group: []buildpack.GroupElement{
							{ID: "A", Version: "v2"},
							{ID: "B", Version: "v2"},
						},
					},
					{
						Group: []buildpack.GroupElement{
							{ID: "C", Version: "v2"},
							{ID: "D", Version: "v2"},
						},
					},
				},
			}
			dirStore.EXPECT().LookupBp("G", "v1").Return(bpG1, nil).AnyTimes()
			bpB2 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v2"}},
			}
			dirStore.EXPECT().LookupBp("B", "v2").Return(bpB2, nil).AnyTimes()
			bpC2 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "C", Version: "v2"}},
			}
			dirStore.EXPECT().LookupBp("C", "v2").Return(bpC2, nil).AnyTimes()
			bpD2 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "D", Version: "v2"}},
			}
			dirStore.EXPECT().LookupBp("D", "v2").Return(bpD2, nil).AnyTimes()

			executor.EXPECT().Detect(bpA1, gomock.Any(), gomock.Any())
			executor.EXPECT().Detect(bpC1, gomock.Any(), gomock.Any())
			executor.EXPECT().Detect(bpB1, gomock.Any(), gomock.Any())

			firstGroup := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"}, // resolver receives homepage
				{ID: "C", Version: "v1", API: "0.2"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			firstResolve := resolver.EXPECT().Resolve(
				firstGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupElement{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			)

			// bpA1 already done
			executor.EXPECT().Detect(bpB2, gomock.Any(), gomock.Any())

			secondGroup := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"},
				{ID: "B", Version: "v2", API: "0.2"},
			}
			secondResolve := resolver.EXPECT().Resolve(
				secondGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupElement{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			).After(firstResolve)

			// bpA1 already done
			executor.EXPECT().Detect(bpC2, gomock.Any(), gomock.Any())
			executor.EXPECT().Detect(bpD2, gomock.Any(), gomock.Any())
			// bpB1 already done

			thirdGroup := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"},
				{ID: "C", Version: "v2", API: "0.2"},
				{ID: "D", Version: "v2", API: "0.2"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			thirdResolve := resolver.EXPECT().Resolve(
				thirdGroup,
				detector.Runs,
			).Return(
				[]buildpack.GroupElement{},
				[]platform.BuildPlanEntry{},
				lifecycle.ErrFailedDetection,
			).After(secondResolve)

			// bpA1 already done
			// bpB1 already done

			fourthGroup := []buildpack.GroupElement{
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
				{Group: []buildpack.GroupElement{{ID: "E", Version: "v1"}}},
			}
			detector.Order = order
			group, plan, err := detector.Detect()
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(group, buildpack.Group{
				Group: []buildpack.GroupElement{
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

		it("converts top level versions to metadata versions", func() {
			bpA1 := &buildpack.BpDescriptor{
				WithAPI:   "0.3",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}},
			}
			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil).AnyTimes()

			bpB1 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}},
			}
			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil).AnyTimes()

			executor.EXPECT().Detect(bpA1, gomock.Any(), gomock.Any())
			executor.EXPECT().Detect(bpB1, gomock.Any(), gomock.Any())

			group := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			resolver.EXPECT().Resolve(group, detector.Runs).Return(group, []platform.BuildPlanEntry{
				{
					Providers: []buildpack.GroupElement{
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
					Providers: []buildpack.GroupElement{
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

			detector.Order = buildpack.Order{{Group: group}}
			found, plan, err := detector.Detect()
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(found, buildpack.Group{Group: group}); s != "" {
				t.Fatalf("Unexpected group:\n%s\n", s)
			}

			if !hasEntries(plan.Entries, []platform.BuildPlanEntry{
				{
					Providers: []buildpack.GroupElement{
						{ID: "A", Version: "v1"},
					},
					Requires: []buildpack.Require{
						{Name: "dep1", Metadata: map[string]interface{}{"version": "some-version"}},
					},
				},
				{
					Providers: []buildpack.GroupElement{
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

		it("updates detect runs for each buildpack", func() {
			bpA1 := &buildpack.BpDescriptor{
				WithAPI:   "0.3",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}},
			}
			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil).AnyTimes()
			executor.EXPECT().Detect(bpA1, gomock.Any(), gomock.Any()).Return(buildpack.DetectOutputs{
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

			bpB1 := &buildpack.BpDescriptor{
				WithAPI:   "0.2",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}},
			}
			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil).AnyTimes()
			bpBerror := errors.New("some-error")
			executor.EXPECT().Detect(bpB1, gomock.Any(), gomock.Any()).Return(buildpack.DetectOutputs{
				Output: []byte("detect out: B@v1\ndetect err: B@v1"),
				Code:   100,
				Err:    bpBerror,
			})

			group := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3"},
				{ID: "B", Version: "v1", API: "0.2"},
			}
			resolver.EXPECT().Resolve(group, detector.Runs).Return(group, []platform.BuildPlanEntry{}, nil)

			detector.Order = buildpack.Order{{Group: group}}
			_, _, err := detector.Detect()
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			bpARun, ok := detector.Runs.Load("Buildpack A@v1")
			if !ok {
				t.Fatalf("missing detection of '%s'", "A@v1")
			}
			if s := cmp.Diff(bpARun, buildpack.DetectOutputs{
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

			bpBRun, ok := detector.Runs.Load("Buildpack B@v1")
			if !ok {
				t.Fatalf("missing detection of '%s'", "B@v1")
			}
			if s := cmp.Diff(bpBRun, buildpack.DetectOutputs{
				Output: []byte("detect out: B@v1\ndetect err: B@v1"),
				Code:   100,
				Err:    bpBerror,
			}, cmp.Comparer(errors.Is)); s != "" {
				t.Fatalf("Unexpected detect run:\n%s\n", s)
			}
		})

		it("preserves 'optional'", func() {
			bpA1 := &buildpack.BpDescriptor{
				WithAPI:   "0.3",
				Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}},
			}
			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil).AnyTimes()
			executor.EXPECT().Detect(bpA1, gomock.Any(), gomock.Any())

			group := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3", Optional: true},
			}
			resolver.EXPECT().Resolve(group, detector.Runs)

			detector.Order = buildpack.Order{{Group: group}}
			_, _, _ = detector.Detect()
		})

		when("resolve errors", func() {
			when("with buildpack error", func() {
				it("returns a buildpack error", func() {
					bpA1 := &buildpack.BpDescriptor{
						WithAPI:   "0.3",
						Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}},
					}
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil).AnyTimes()
					executor.EXPECT().Detect(bpA1, gomock.Any(), gomock.Any())

					group := []buildpack.GroupElement{
						{ID: "A", Version: "v1", API: "0.3"},
					}
					resolver.EXPECT().Resolve(group, detector.Runs).Return(
						[]buildpack.GroupElement{},
						[]platform.BuildPlanEntry{},
						lifecycle.ErrBuildpack,
					)

					detector.Order = buildpack.Order{{Group: group}}
					_, _, err := detector.Detect()
					if err, ok := err.(*buildpack.Error); !ok || err.Type != buildpack.ErrTypeBuildpack {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
				})
			})

			when("with detect error", func() {
				it("returns a detect error", func() {
					bpA1 := &buildpack.BpDescriptor{
						WithAPI:   "0.3",
						Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}},
					}
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil).AnyTimes()
					executor.EXPECT().Detect(bpA1, gomock.Any(), gomock.Any())

					group := []buildpack.GroupElement{
						{ID: "A", Version: "v1", API: "0.3"},
					}
					resolver.EXPECT().Resolve(group, detector.Runs).Return(
						[]buildpack.GroupElement{},
						[]platform.BuildPlanEntry{},
						lifecycle.ErrFailedDetection,
					)

					detector.Order = buildpack.Order{{Group: group}}
					_, _, err := detector.Detect()
					if err, ok := err.(*buildpack.Error); !ok || err.Type != buildpack.ErrTypeFailedDetection {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
				})
			})

			when("A Target is provided from AnalyzeMD", func() {
				it("errors if the buildpacks don't share that target arch/os", func() {
					detector.AnalyzeMD.API = "0.12"
					detector.AnalyzeMD.RunImage.Target.Os = "MacOS"
					detector.AnalyzeMD.RunImage.Target.Arch = "ARM64"
					detector.AnalyzeMD.RunImage.Target.Distribution = &buildpack.DistributionMetadata{Name: "MacOS", Version: "some kind of big cat"}

					bpA1 := &buildpack.BpDescriptor{
						WithAPI:   "0.12",
						Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}},
						Targets: []buildpack.TargetMetadata{
							{TargetPartial: buildpack.TargetPartial{Arch: "P6", ArchVariant: "Pentium Pro", Os: "Win95"},
								Distributions: []buildpack.DistributionMetadata{
									{Name: "Windows 95", Version: "OSR1"}, {Name: "Windows 95", Version: "OSR2.5"}}},
							{TargetPartial: buildpack.TargetPartial{Arch: "Pentium M", Os: "Win98"},
								Distributions: []buildpack.DistributionMetadata{{Name: "Windows 2000", Version: "Server"}}},
						},
					}
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil).AnyTimes()

					resolver.EXPECT().Resolve(gomock.Any(), gomock.Any()).DoAndReturn(
						func(done []buildpack.GroupElement, detectRuns *sync.Map) ([]buildpack.GroupElement, []platform.BuildPlanEntry, error) {
							h.AssertEq(t, len(done), 1)
							val, ok := detectRuns.Load("Buildpack A@v1")
							h.AssertEq(t, ok, true)
							outs := val.(buildpack.DetectOutputs)
							h.AssertEq(t, outs.Code, -1)
							h.AssertStringContains(t, outs.Err.Error(), "unable to satisfy Target OS/Arch constriaints")
							return []buildpack.GroupElement{}, []platform.BuildPlanEntry{}, nil
						})

					group := []buildpack.GroupElement{
						{ID: "A", Version: "v1", API: "0.3"},
					}
					detector.Order = buildpack.Order{{Group: group}}
					_, _, err := detector.Detect() // even though the returns from this are directly from the mock above, if we don't check the returns the linter declares we've done it wrong and fails on the lack of assertions.
					h.AssertNil(t, err)
				})
				it("totally works if the constraints are met", func() {
					detector.AnalyzeMD.RunImage.Target.Os = "MacOS"
					detector.AnalyzeMD.RunImage.Target.Arch = "ARM64"
					detector.AnalyzeMD.RunImage.Target.Distribution = &buildpack.DistributionMetadata{Name: "MacOS", Version: "snow cheetah"}

					bpA1 := &buildpack.BpDescriptor{
						WithAPI:   "0.12",
						Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}},
						Targets: []buildpack.TargetMetadata{
							{TargetPartial: buildpack.TargetPartial{Arch: "P6", ArchVariant: "Pentium Pro", Os: "Win95"},
								Distributions: []buildpack.DistributionMetadata{
									{Name: "Windows 95", Version: "OSR1"}, {Name: "Windows 95", Version: "OSR2.5"}}},
							{TargetPartial: buildpack.TargetPartial{Arch: "ARM64", Os: "MacOS"}, Distributions: []buildpack.DistributionMetadata{{Name: "MacOS", Version: "snow cheetah"}}}},
					}
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil).AnyTimes()
					executor.EXPECT().Detect(bpA1, gomock.Any(), gomock.Any())

					group := []buildpack.GroupElement{
						{ID: "A", Version: "v1", API: "0.12"},
					}
					// the most meaningful assertion in this test is that `group` is the first argument to Resolve, meaning that the buildpack matched.
					resolver.EXPECT().Resolve(group, detector.Runs).Return(
						[]buildpack.GroupElement{},
						[]platform.BuildPlanEntry{},
						nil,
					)

					detector.Order = buildpack.Order{{Group: group}}
					_, _, err := detector.Detect()
					h.AssertNil(t, err)
				})
			})
		})

		when("there are extensions", func() {
			it("selects the first passing group", func() {
				// This test doesn't use gomock.InOrder() because each call to Detect() happens in a go func.
				// The order that other calls are written in is the order that they happen in.

				bpA1 := &buildpack.BpDescriptor{
					WithAPI:   "0.8",
					Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}},
				}
				dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil).AnyTimes()
				bpB1 := &buildpack.BpDescriptor{
					WithAPI:   "0.8",
					Buildpack: buildpack.BpInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}},
				}
				dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil).AnyTimes()
				extA1 := &buildpack.ExtDescriptor{
					WithAPI:   "0.9",
					Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}},
				}
				dirStore.EXPECT().LookupExt("A", "v1").Return(extA1, nil).AnyTimes()
				extB1 := &buildpack.ExtDescriptor{
					WithAPI:   "0.9",
					Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}},
				}
				dirStore.EXPECT().LookupExt("B", "v1").Return(extB1, nil).AnyTimes()

				executor.EXPECT().Detect(extA1, gomock.Any(), gomock.Any())
				executor.EXPECT().Detect(bpA1, gomock.Any(), gomock.Any())

				firstGroup := []buildpack.GroupElement{
					{ID: "A", Version: "v1", API: "0.9", Extension: true, Optional: true},
					{ID: "A", Version: "v1", API: "0.8"},
				}
				firstResolve := resolver.EXPECT().Resolve(
					firstGroup,
					detector.Runs,
				).Return(
					[]buildpack.GroupElement{},
					[]platform.BuildPlanEntry{},
					lifecycle.ErrFailedDetection,
				)

				executor.EXPECT().Detect(extB1, gomock.Any(), gomock.Any())
				// bpA1 already done

				secondGroup := []buildpack.GroupElement{
					{ID: "B", Version: "v1", API: "0.9", Extension: true, Optional: true},
					{ID: "A", Version: "v1", API: "0.8"},
				}
				secondResolve := resolver.EXPECT().Resolve(
					secondGroup,
					detector.Runs,
				).Return(
					[]buildpack.GroupElement{},
					[]platform.BuildPlanEntry{},
					lifecycle.ErrFailedDetection,
				).After(firstResolve)

				// bpA1 already done

				thirdGroup := []buildpack.GroupElement{
					{ID: "A", Version: "v1", API: "0.8"},
				}
				thirdResolve := resolver.EXPECT().Resolve(
					thirdGroup,
					detector.Runs,
				).Return(
					[]buildpack.GroupElement{},
					[]platform.BuildPlanEntry{},
					lifecycle.ErrFailedDetection,
				).After(secondResolve)

				// extA1 already done
				executor.EXPECT().Detect(bpB1, gomock.Any(), gomock.Any())

				fourthGroup := []buildpack.GroupElement{
					{ID: "A", Version: "v1", API: "0.9", Extension: true, Optional: true},
					{ID: "B", Version: "v1", API: "0.8"},
				}
				fourthResolve := resolver.EXPECT().Resolve(
					fourthGroup,
					detector.Runs,
				).Return(
					[]buildpack.GroupElement{},
					[]platform.BuildPlanEntry{},
					lifecycle.ErrFailedDetection,
				).After(thirdResolve)

				// extB1 already done
				// bpB1 already done

				fifthGroup := []buildpack.GroupElement{
					{ID: "B", Version: "v1", API: "0.9", Extension: true, Optional: true},
					{ID: "B", Version: "v1", API: "0.8"},
				}
				resolver.EXPECT().Resolve(
					fifthGroup,
					detector.Runs,
				).Return(
					[]buildpack.GroupElement{
						{ID: "B", Version: "v1", API: "0.9", Extension: true}, // optional removed
						{ID: "B", Version: "v1", API: "0.8"},
					},
					[]platform.BuildPlanEntry{},
					nil,
				).After(fourthResolve)

				orderBp := buildpack.Order{
					{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
					{Group: []buildpack.GroupElement{{ID: "B", Version: "v1"}}},
				}
				orderExt := buildpack.Order{
					{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
					{Group: []buildpack.GroupElement{{ID: "B", Version: "v1"}}},
				}

				detector.Order = lifecycle.PrependExtensions(orderBp, orderExt)
				group, _, err := detector.Detect()
				h.AssertNil(t, err)

				h.AssertEq(t, group.Group, []buildpack.GroupElement{{ID: "B", Version: "v1", API: "0.8"}})
				h.AssertEq(t, group.GroupExtensions, []buildpack.GroupElement{{ID: "B", Version: "v1", API: "0.9"}})
			})
		})
	})

	when(".Resolve", func() {
		var (
			resolver   *lifecycle.DefaultDetectResolver
			logHandler *memory.Handler
			logger     *log.DefaultLogger
		)

		it.Before(func() {
			logHandler = memory.New()
			logger = &log.DefaultLogger{Logger: &apexlog.Logger{Handler: logHandler}}
			resolver = lifecycle.NewDefaultDetectResolver(logger)
		})

		it("fails if the group is empty", func() {
			_, _, err := resolver.Resolve([]buildpack.GroupElement{}, &sync.Map{})
			if err != lifecycle.ErrFailedDetection {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}

			if s := cmp.Diff(h.AllLogs(logHandler),
				"======== Results ========\n"+
					"fail: no viable buildpacks in group\n",
			); s != "" {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		it("fails if the group has no viable buildpacks, even if no required buildpacks fail", func() {
			group := []buildpack.GroupElement{
				{ID: "A", Version: "v1", Optional: true},
				{ID: "B", Version: "v1", Optional: true},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("Buildpack A@v1", buildpack.DetectOutputs{
				Code: 100,
			})
			detectRuns.Store("Buildpack B@v1", buildpack.DetectOutputs{
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
					"fail: no viable buildpacks in group\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		when("there are extensions", func() {
			it("fails if the group has no viable buildpacks, even if no required buildpacks fail", func() {
				group := []buildpack.GroupElement{
					{ID: "A", Version: "v1", Optional: true},
					{ID: "B", Version: "v1", Extension: true, Optional: true},
				}

				detectRuns := &sync.Map{}
				detectRuns.Store("Buildpack A@v1", buildpack.DetectOutputs{
					Code: 100,
				})
				detectRuns.Store("Extension B@v1", buildpack.DetectOutputs{
					Code: 0,
				})

				_, _, err := resolver.Resolve(group, detectRuns)
				if err != lifecycle.ErrFailedDetection {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := h.AllLogs(logHandler); !strings.HasSuffix(s,
					"======== Results ========\n"+
						"skip: A@v1\n"+
						"pass: B@v1\n"+
						"fail: no viable buildpacks in group\n",
				) {
					t.Fatalf("Unexpected log:\n%s\n", s)
				}
			})
		})

		it("fails with specific error if any bp detect fails in an unexpected way", func() {
			group := []buildpack.GroupElement{
				{ID: "A", Version: "v1", Optional: false},
				{ID: "B", Version: "v1", Optional: false},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("Buildpack A@v1", buildpack.DetectOutputs{
				Code: 0,
			})
			detectRuns.Store("Buildpack B@v1", buildpack.DetectOutputs{
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

		when("log output", func() {
			it.Before(func() {
				h.AssertNil(t, logger.SetLevel("info"))
			})

			it("outputs detect pass and fail as debug level", func() {
				group := []buildpack.GroupElement{
					{ID: "A", Version: "v1", Optional: false},
					{ID: "B", Version: "v1", Optional: false},
				}

				detectRuns := &sync.Map{}
				detectRuns.Store("Buildpack A@v1", buildpack.DetectOutputs{
					Code: 0,
				})
				detectRuns.Store("Buildpack B@v1", buildpack.DetectOutputs{
					Code: 100,
				})

				_, _, err := resolver.Resolve(group, detectRuns)
				if err != lifecycle.ErrFailedDetection {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}

				if s := h.AllLogs(logHandler); s != "" {
					t.Fatalf("Unexpected log:\n%s\n", s)
				}
			})

			it("outputs detect errors as info level", func() {
				group := []buildpack.GroupElement{
					{ID: "A", Version: "v1", Optional: false},
					{ID: "B", Version: "v1", Optional: false},
				}

				detectRuns := &sync.Map{}
				detectRuns.Store("Buildpack A@v1", buildpack.DetectOutputs{
					Code: 0,
				})
				detectRuns.Store("Buildpack B@v1", buildpack.DetectOutputs{
					Output: []byte("detect out: B@v1\ndetect err: B@v1"),
					Code:   127,
				})

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
		})

		it("returns a build plan with matched dependencies", func() {
			group := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"},
				{ID: "C", Version: "v2", API: "0.2"},
				{ID: "D", Version: "v2", API: "0.2"},
				{ID: "B", Version: "v1", API: "0.2"},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("Buildpack A@v1", buildpack.DetectOutputs{
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
			detectRuns.Store("Buildpack B@v1", buildpack.DetectOutputs{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Requires: []buildpack.Require{
							{Name: "dep1"},
							{Name: "dep2"},
						},
					},
				},
			})
			detectRuns.Store("Buildpack C@v2", buildpack.DetectOutputs{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep1"},
							{Name: "dep2"},
						},
					},
				},
			})
			detectRuns.Store("Buildpack D@v2", buildpack.DetectOutputs{
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
					Providers: []buildpack.GroupElement{
						{ID: "A", Version: "v1"},
						{ID: "C", Version: "v2"},
					},
					Requires: []buildpack.Require{{Name: "dep1"}, {Name: "dep1"}},
				},
				{
					Providers: []buildpack.GroupElement{
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

		it("fails if all requires are not provided first", func() {
			group := []buildpack.GroupElement{
				{ID: "A", Version: "v1", Optional: true},
				{ID: "B", Version: "v1"},
				{ID: "C", Version: "v1"},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("Buildpack A@v1", buildpack.DetectOutputs{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep1"},
						},
					},
				},
				Code: 100,
			})
			detectRuns.Store("Buildpack B@v1", buildpack.DetectOutputs{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Requires: []buildpack.Require{
							{Name: "dep1"},
						},
					},
				},
			})
			detectRuns.Store("Buildpack C@v1", buildpack.DetectOutputs{
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

		it("fails if all provides are not required after", func() {
			group := []buildpack.GroupElement{
				{ID: "A", Version: "v1"},
				{ID: "B", Version: "v1"},
				{ID: "C", Version: "v1", Optional: true},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("Buildpack A@v1", buildpack.DetectOutputs{
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
			detectRuns.Store("Buildpack B@v1", buildpack.DetectOutputs{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Provides: []buildpack.Provide{
							{Name: "dep1"},
						},
					},
				},
			})
			detectRuns.Store("Buildpack C@v1", buildpack.DetectOutputs{
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

		it("succeeds if unmet provides/requires are optional", func() {
			group := []buildpack.GroupElement{
				{ID: "A", Version: "v1", Optional: true},
				{ID: "B", Version: "v1", API: "0.2"},
				{ID: "C", Version: "v1", Optional: true},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("Buildpack A@v1", buildpack.DetectOutputs{
				BuildPlan: buildpack.BuildPlan{
					PlanSections: buildpack.PlanSections{
						Requires: []buildpack.Require{
							{Name: "dep-missing"},
						},
					},
				},
			})
			detectRuns.Store("Buildpack B@v1", buildpack.DetectOutputs{
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
			detectRuns.Store("Buildpack C@v1", buildpack.DetectOutputs{
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

			if s := cmp.Diff(found, []buildpack.GroupElement{
				{ID: "B", Version: "v1", API: "0.2"},
			}); s != "" {
				t.Fatalf("Unexpected group:\n%s\n", s)
			}

			if !hasEntries(entries, []platform.BuildPlanEntry{
				{
					Providers: []buildpack.GroupElement{{ID: "B", Version: "v1"}},
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

		it("falls back to alternate build plans", func() {
			group := []buildpack.GroupElement{
				{ID: "A", Version: "v1", Optional: true, API: "0.3", Homepage: "Buildpack A Homepage"},
				{ID: "B", Version: "v1", Optional: true, API: "0.2"},
				{ID: "C", Version: "v1", API: "0.2"},
				{ID: "D", Version: "v1", Optional: true},
			}

			detectRuns := &sync.Map{}
			detectRuns.Store("Buildpack A@v1", buildpack.DetectOutputs{
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
			detectRuns.Store("Buildpack B@v1", buildpack.DetectOutputs{
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
			detectRuns.Store("Buildpack C@v1", buildpack.DetectOutputs{
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
			detectRuns.Store("Buildpack D@v1", buildpack.DetectOutputs{
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

			if s := cmp.Diff(found, []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"},
				{ID: "B", Version: "v1", API: "0.2"},
				{ID: "C", Version: "v1", API: "0.2"},
			}); s != "" {
				t.Fatalf("Unexpected group:\n%s\n", s)
			}

			if !hasEntries(entries, []platform.BuildPlanEntry{
				{
					Providers: []buildpack.GroupElement{{ID: "A", Version: "v1"}},
					Requires:  []buildpack.Require{{Name: "dep1-present"}},
				},
				{
					Providers: []buildpack.GroupElement{{ID: "C", Version: "v1"}},
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

	when("#PrependExtensions", func() {
		it("prepends the extensions order to each group in the buildpacks order", func() {
			orderBp := buildpack.Order{
				buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
				buildpack.Group{Group: []buildpack.GroupElement{{ID: "B", Version: "v1"}}},
			}
			orderExt := buildpack.Order{
				buildpack.Group{Group: []buildpack.GroupElement{{ID: "C", Version: "v1"}}},
				buildpack.Group{Group: []buildpack.GroupElement{{ID: "D", Version: "v1"}}},
			}
			expectedOrderExt := buildpack.Order{
				buildpack.Group{Group: []buildpack.GroupElement{{ID: "C", Version: "v1", Extension: true, Optional: true}}},
				buildpack.Group{Group: []buildpack.GroupElement{{ID: "D", Version: "v1", Extension: true, Optional: true}}},
			}

			newOrder := lifecycle.PrependExtensions(orderBp, orderExt)

			t.Log("returns the modified order")
			if s := cmp.Diff(newOrder, buildpack.Order{
				buildpack.Group{
					Group: []buildpack.GroupElement{
						{OrderExtensions: expectedOrderExt},
						{ID: "A", Version: "v1"},
					},
				},
				buildpack.Group{
					Group: []buildpack.GroupElement{
						{OrderExtensions: expectedOrderExt},
						{ID: "B", Version: "v1"},
					},
				},
			}); s != "" {
				t.Fatalf("Unexpected:\n%s\n", s)
			}

			t.Log("does not modify the originally provided order")
			if s := cmp.Diff(orderBp, buildpack.Order{
				buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
				buildpack.Group{Group: []buildpack.GroupElement{{ID: "B", Version: "v1"}}},
			}); s != "" {
				t.Fatalf("Unexpected:\n%s\n", s)
			}
		})

		when("the extensions order is empty", func() {
			it("returns the originally provided order", func() {
				orderBp := buildpack.Order{
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "B", Version: "v1"}}},
				}

				newOrder := lifecycle.PrependExtensions(orderBp, nil)

				if s := cmp.Diff(newOrder, buildpack.Order{
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "B", Version: "v1"}}},
				}); s != "" {
					t.Fatalf("Unexpected:\n%s\n", s)
				}
			})
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
