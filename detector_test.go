package lifecycle_test

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
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
	when("#NewDetector", func() {
		var (
			detectorFactory   *lifecycle.DetectorFactory
			fakeAPIVerifier   *testmock.MockAPIVerifier
			fakeConfigHandler *testmock.MockConfigHandler
			fakeDirStore      *testmock.MockDirStore
			logger            *log.Logger
			mockController    *gomock.Controller
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			fakeAPIVerifier = testmock.NewMockAPIVerifier(mockController)
			fakeConfigHandler = testmock.NewMockConfigHandler(mockController)
			fakeDirStore = testmock.NewMockDirStore(mockController)
			logger = &log.Logger{Handler: &discard.Handler{}}
		})

		it.After(func() {
			mockController.Finish()
		})

		when("platform api >= 0.10", func() { // TODO: change to pre-release api in https://github.com/buildpacks/lifecycle/issues/459
			it.Before(func() {
				detectorFactory = lifecycle.NewDetectorFactory(api.Platform.Latest(), fakeAPIVerifier, fakeConfigHandler, fakeDirStore)
			})

			it("configures the detector", func() {
				order := buildpack.Order{
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
				}
				fakeConfigHandler.EXPECT().ReadOrder("some-order-path").Return(order, nil, nil)

				t.Log("verifies buildpack apis")
				bpA1 := testmock.NewMockBuildModule(mockController)
				fakeDirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
				bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2"})
				fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindBuildpack, "A@v1", "0.2")

				detector, err := detectorFactory.NewDetector("some-app-dir", "some-order-path", "some-platform-dir", logger)
				h.AssertNil(t, err)

				h.AssertEq(t, detector.AppDir, "some-app-dir")
				h.AssertNotNil(t, detector.DirStore)
				h.AssertEq(t, detector.Logger, logger)
				h.AssertEq(t, detector.Order, order)
				h.AssertEq(t, detector.PlatformDir, "some-platform-dir")
				_, ok := detector.Resolver.(*lifecycle.DefaultResolver)
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
						buildpack.Group{Group: []buildpack.GroupElement{{ID: "C", Version: "v1"}}},
						buildpack.Group{Group: []buildpack.GroupElement{{ID: "D", Version: "v1"}}},
					}
					expectedOrder := buildpack.Order{
						buildpack.Group{
							Group: []buildpack.GroupElement{
								{OrderExt: buildpack.Order{
									buildpack.Group{Group: []buildpack.GroupElement{{ID: "C", Version: "v1", Extension: true, Optional: true}}},
									buildpack.Group{Group: []buildpack.GroupElement{{ID: "D", Version: "v1", Extension: true, Optional: true}}},
								}},
								{ID: "A", Version: "v1"},
							},
						},
						buildpack.Group{
							Group: []buildpack.GroupElement{
								{OrderExt: buildpack.Order{
									buildpack.Group{Group: []buildpack.GroupElement{{ID: "C", Version: "v1", Extension: true, Optional: true}}},
									buildpack.Group{Group: []buildpack.GroupElement{{ID: "D", Version: "v1", Extension: true, Optional: true}}},
								}},
								{ID: "B", Version: "v1"},
							},
						},
					}
					fakeConfigHandler.EXPECT().ReadOrder("some-order-path").Return(orderBp, orderExt, nil)

					t.Log("verifies buildpack apis")
					bpA1 := testmock.NewMockBuildModule(mockController)
					bpB1 := testmock.NewMockBuildModule(mockController)
					extC1 := testmock.NewMockBuildModule(mockController)
					extD1 := testmock.NewMockBuildModule(mockController)
					fakeDirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
					bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2"})
					fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindBuildpack, "A@v1", "0.2")
					fakeDirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
					bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2"})
					fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindBuildpack, "B@v1", "0.2")
					fakeDirStore.EXPECT().LookupExt("C", "v1").Return(extC1, nil)
					extC1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.10"})
					fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindExtension, "C@v1", "0.10")
					fakeDirStore.EXPECT().LookupExt("D", "v1").Return(extD1, nil)
					extD1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.10"})
					fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindExtension, "D@v1", "0.10")

					detector, err := detectorFactory.NewDetector("some-app-dir", "some-order-path", "some-platform-dir", logger)
					h.AssertNil(t, err)

					h.AssertEq(t, detector.AppDir, "some-app-dir")
					h.AssertNotNil(t, detector.DirStore)
					h.AssertEq(t, detector.Logger, logger)
					h.AssertEq(t, detector.Order, expectedOrder)
					h.AssertEq(t, detector.PlatformDir, "some-platform-dir")
					_, ok := detector.Resolver.(*lifecycle.DefaultResolver)
					h.AssertEq(t, ok, true)
					h.AssertNotNil(t, detector.Runs)
				})
			})
		})

		when("platform api < 0.10", func() { // TODO: change to pre-release api in https://github.com/buildpacks/lifecycle/issues/459
			it.Before(func() {
				detectorFactory = lifecycle.NewDetectorFactory(api.MustParse("0.9"), fakeAPIVerifier, fakeConfigHandler, fakeDirStore)
			})

			it("configures the detector", func() {
				order := buildpack.Order{
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
				}
				fakeConfigHandler.EXPECT().ReadOrder("some-order-path").Return(order, nil, nil)

				t.Log("verifies buildpack apis")
				bpA1 := testmock.NewMockBuildModule(mockController)
				fakeDirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
				bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2"})
				fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindBuildpack, "A@v1", "0.2")

				detector, err := detectorFactory.NewDetector("some-app-dir", "some-order-path", "some-platform-dir", logger)
				h.AssertNil(t, err)

				h.AssertEq(t, detector.AppDir, "some-app-dir")
				h.AssertNotNil(t, detector.DirStore)
				h.AssertEq(t, detector.Logger, logger)
				h.AssertEq(t, detector.Order, order)
				h.AssertEq(t, detector.PlatformDir, "some-platform-dir")
				_, ok := detector.Resolver.(*lifecycle.DefaultResolver)
				h.AssertEq(t, ok, true)
				h.AssertNotNil(t, detector.Runs)
			})

			when("there are extensions", func() {
				it("ignores them", func() {
					orderBp := buildpack.Order{
						buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
						buildpack.Group{Group: []buildpack.GroupElement{{ID: "B", Version: "v1"}}},
					}
					orderExt := buildpack.Order{
						buildpack.Group{Group: []buildpack.GroupElement{{ID: "C", Version: "v1"}}},
						buildpack.Group{Group: []buildpack.GroupElement{{ID: "D", Version: "v1"}}},
					}
					fakeConfigHandler.EXPECT().ReadOrder("some-order-path").Return(orderBp, orderExt, nil)

					t.Log("verifies buildpack apis")
					bpA1 := testmock.NewMockBuildModule(mockController)
					bpB1 := testmock.NewMockBuildModule(mockController)
					fakeDirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
					bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2"})
					fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindBuildpack, "A@v1", "0.2")
					fakeDirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
					bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2"})
					fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindBuildpack, "B@v1", "0.2")

					detector, err := detectorFactory.NewDetector("some-app-dir", "some-order-path", "some-platform-dir", logger)
					h.AssertNil(t, err)

					h.AssertEq(t, detector.AppDir, "some-app-dir")
					h.AssertNotNil(t, detector.DirStore)
					h.AssertEq(t, detector.Logger, logger)
					h.AssertEq(t, detector.Order, orderBp)
					h.AssertEq(t, detector.PlatformDir, "some-platform-dir")
					_, ok := detector.Resolver.(*lifecycle.DefaultResolver)
					h.AssertEq(t, ok, true)
					h.AssertNotNil(t, detector.Runs)
				})
			})
		})
	})

	when(".Detect", func() {
		var (
			detector *lifecycle.Detector
			mockCtrl *gomock.Controller
			resolver *testmock.MockResolver
			dirStore *testmock.MockDirStore
		)

		it.Before(func() {
			mockCtrl = gomock.NewController(t)
			dirStore = testmock.NewMockDirStore(mockCtrl)
			resolver = testmock.NewMockResolver(mockCtrl)

			detector = &lifecycle.Detector{
				DirStore: dirStore,
				Logger:   nil,
				Resolver: resolver,
				Runs:     &sync.Map{},
			}
		})

		it.After(func() {
			mockCtrl.Finish()
		})

		it("should expand order-containing buildpack IDs", func() {
			// This test doesn't use gomock.InOrder() because each call to Detect() happens in a go func.
			// The order that other calls are written in is the order that they happen in.

			bpE1 := testmock.NewMockBuildModule(mockCtrl)
			bpA1 := testmock.NewMockBuildModule(mockCtrl)
			bpF1 := testmock.NewMockBuildModule(mockCtrl)
			bpC1 := testmock.NewMockBuildModule(mockCtrl)
			bpB1 := testmock.NewMockBuildModule(mockCtrl)
			bpG1 := testmock.NewMockBuildModule(mockCtrl)
			bpB2 := testmock.NewMockBuildModule(mockCtrl)
			bpC2 := testmock.NewMockBuildModule(mockCtrl)
			bpD2 := testmock.NewMockBuildModule(mockCtrl)
			bpD1 := testmock.NewMockBuildModule(mockCtrl)

			dirStore.EXPECT().LookupBp("E", "v1").Return(bpE1, nil)
			bpE1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
				API:       "0.2",
				Buildpack: buildpack.Info{ID: "E", Version: "v1"},
				Order: []buildpack.Group{
					{
						Group: []buildpack.GroupElement{
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
			})

			dirStore.EXPECT().LookupBp("C", "v1").Return(bpC1, nil)
			bpC1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "C", Version: "v1"}})
			bpC1.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})
			bpB1.EXPECT().Detect(gomock.Any(), gomock.Any())

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

			dirStore.EXPECT().LookupBp("G", "v1").Return(bpG1, nil)
			bpG1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
				API:       "0.2",
				Buildpack: buildpack.Info{ID: "G", Version: "v1"},
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
			})

			dirStore.EXPECT().LookupBp("B", "v2").Return(bpB2, nil)
			bpB2.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v2"}})
			bpB2.EXPECT().Detect(gomock.Any(), gomock.Any())

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

			dirStore.EXPECT().LookupBp("C", "v2").Return(bpC2, nil)
			bpC2.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "C", Version: "v2"}})
			bpC2.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("D", "v2").Return(bpD2, nil)
			bpD2.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "D", Version: "v2"}})
			bpD2.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})

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

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})

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

			dirStore.EXPECT().LookupBp("D", "v1").Return(bpD1, nil)
			bpD1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "D", Version: "v1"}})
			bpD1.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})

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

		it("should select the first passing group", func() {
			// This test doesn't use gomock.InOrder() because each call to Detect() happens in a go func.
			// The order that other calls are written in is the order that they happen in.

			bpE1 := testmock.NewMockBuildModule(mockCtrl)
			bpA1 := testmock.NewMockBuildModule(mockCtrl)
			bpF1 := testmock.NewMockBuildModule(mockCtrl)
			bpC1 := testmock.NewMockBuildModule(mockCtrl)
			bpB1 := testmock.NewMockBuildModule(mockCtrl)
			bpG1 := testmock.NewMockBuildModule(mockCtrl)
			bpB2 := testmock.NewMockBuildModule(mockCtrl)
			bpC2 := testmock.NewMockBuildModule(mockCtrl)
			bpD2 := testmock.NewMockBuildModule(mockCtrl)

			dirStore.EXPECT().LookupBp("E", "v1").Return(bpE1, nil)
			bpE1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
				API:       "0.2",
				Buildpack: buildpack.Info{ID: "E", Version: "v1"},
				Order: []buildpack.Group{
					{
						Group: []buildpack.GroupElement{
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
			})

			dirStore.EXPECT().LookupBp("C", "v1").Return(bpC1, nil)
			bpC1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "C", Version: "v1"}})
			bpC1.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})
			bpB1.EXPECT().Detect(gomock.Any(), gomock.Any())

			firstGroup := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.3", Homepage: "Buildpack A Homepage"},
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

			dirStore.EXPECT().LookupBp("G", "v1").Return(bpG1, nil)
			bpG1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
				API:       "0.2",
				Buildpack: buildpack.Info{ID: "G", Version: "v1"},
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
			})

			dirStore.EXPECT().LookupBp("B", "v2").Return(bpB2, nil)
			bpB2.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v2"}})
			bpB2.EXPECT().Detect(gomock.Any(), gomock.Any())

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

			dirStore.EXPECT().LookupBp("C", "v2").Return(bpC2, nil)
			bpC2.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "C", Version: "v2"}})
			bpC2.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("D", "v2").Return(bpD2, nil)
			bpD2.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "D", Version: "v2"}})
			bpD2.EXPECT().Detect(gomock.Any(), gomock.Any())

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})

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

			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})

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

		it("should convert top level versions to metadata versions", func() {
			bpA1 := testmock.NewMockBuildModule(mockCtrl)
			dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
			bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.3", Buildpack: buildpack.Info{ID: "A", Version: "v1"}})
			bpA1.EXPECT().Detect(gomock.Any(), gomock.Any())

			bpB1 := testmock.NewMockBuildModule(mockCtrl)
			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})
			bpB1.EXPECT().Detect(gomock.Any(), gomock.Any())

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

		it("should update detect runs for each buildpack", func() {
			bpA1 := testmock.NewMockBuildModule(mockCtrl)
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

			bpB1 := testmock.NewMockBuildModule(mockCtrl)
			dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
			bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.2", Buildpack: buildpack.Info{ID: "B", Version: "v1"}})
			bpBerror := errors.New("some-error")
			bpB1.EXPECT().Detect(gomock.Any(), gomock.Any()).Return(buildpack.DetectRun{
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
					bpA1 := testmock.NewMockBuildModule(mockCtrl)
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
					bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.3", Buildpack: buildpack.Info{ID: "A", Version: "v1"}})
					bpA1.EXPECT().Detect(gomock.Any(), gomock.Any())

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
					bpA1 := testmock.NewMockBuildModule(mockCtrl)
					dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
					bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{API: "0.3", Buildpack: buildpack.Info{ID: "A", Version: "v1"}})
					bpA1.EXPECT().Detect(gomock.Any(), gomock.Any())

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
		})

		when("there are extensions", func() {
			it("selects the first passing group", func() {
				// This test doesn't use gomock.InOrder() because each call to Detect() happens in a go func.
				// The order that other calls are written in is the order that they happen in.

				bpA1 := testmock.NewMockBuildModule(mockCtrl)
				bpB1 := testmock.NewMockBuildModule(mockCtrl)
				extC1 := testmock.NewMockBuildModule(mockCtrl)
				extD1 := testmock.NewMockBuildModule(mockCtrl)

				// first group

				// process C@v1
				dirStore.EXPECT().LookupExt("C", "v1").Return(extC1, nil)
				extC1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.9",
					Extension: buildpack.Info{ID: "C", Version: "v1"},
				})
				extC1.EXPECT().Detect(gomock.Any(), gomock.Any())

				// process A@v1
				dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
				bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.8",
					Buildpack: buildpack.Info{ID: "A", Version: "v1"},
				})
				bpA1.EXPECT().Detect(gomock.Any(), gomock.Any())

				// try resolve
				firstGroup := []buildpack.GroupElement{
					{ID: "C", Version: "v1", API: "0.9", Extension: true, Optional: true},
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

				// second group

				// process D@v1
				dirStore.EXPECT().LookupExt("D", "v1").Return(extD1, nil)
				extD1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.9",
					Extension: buildpack.Info{ID: "D", Version: "v1"},
				})
				extD1.EXPECT().Detect(gomock.Any(), gomock.Any())

				// process A@v1
				dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
				bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.8",
					Buildpack: buildpack.Info{ID: "A", Version: "v1"},
				})

				// try resolve
				secondGroup := []buildpack.GroupElement{
					{ID: "D", Version: "v1", API: "0.9", Extension: true, Optional: true},
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

				// third group

				// process A@v1
				dirStore.EXPECT().LookupBp("A", "v1").Return(bpA1, nil)
				bpA1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.8",
					Buildpack: buildpack.Info{ID: "A", Version: "v1"},
				})

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

				// fourth group

				// process C@v1
				dirStore.EXPECT().LookupExt("C", "v1").Return(extC1, nil)
				extC1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.9",
					Extension: buildpack.Info{ID: "C", Version: "v1"},
				})

				// process B@v1
				dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
				bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.8",
					Buildpack: buildpack.Info{ID: "B", Version: "v1"},
				})
				bpB1.EXPECT().Detect(gomock.Any(), gomock.Any())

				// try resolve
				fourthGroup := []buildpack.GroupElement{
					{ID: "C", Version: "v1", API: "0.9", Extension: true, Optional: true},
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

				// fifth group

				// process D@v1
				dirStore.EXPECT().LookupExt("D", "v1").Return(extD1, nil)
				extD1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.9",
					Extension: buildpack.Info{ID: "D", Version: "v1"},
				})

				// process B@v1
				dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
				bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.8",
					Buildpack: buildpack.Info{ID: "B", Version: "v1"},
				})

				fifthGroup := []buildpack.GroupElement{
					{ID: "D", Version: "v1", API: "0.9", Extension: true, Optional: true},
					{ID: "B", Version: "v1", API: "0.8"},
				}
				fifthResolve := resolver.EXPECT().Resolve(
					fifthGroup,
					detector.Runs,
				).Return(
					[]buildpack.GroupElement{},
					[]platform.BuildPlanEntry{},
					lifecycle.ErrFailedDetection,
				).After(fourthResolve)

				// sixth group

				// process B@v1
				dirStore.EXPECT().LookupBp("B", "v1").Return(bpB1, nil)
				bpB1.EXPECT().ConfigFile().Return(&buildpack.Descriptor{
					API:       "0.8",
					Buildpack: buildpack.Info{ID: "B", Version: "v1"},
				})

				sixthGroup := []buildpack.GroupElement{
					{ID: "B", Version: "v1", API: "0.8"},
				}
				resolver.EXPECT().Resolve(
					sixthGroup,
					detector.Runs,
				).Return(
					sixthGroup,
					[]platform.BuildPlanEntry{},
					nil,
				).After(fifthResolve)

				orderBp := buildpack.Order{
					{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
					{Group: []buildpack.GroupElement{{ID: "B", Version: "v1"}}},
				}
				orderExt := buildpack.Order{
					{Group: []buildpack.GroupElement{{ID: "C", Version: "v1"}}},
					{Group: []buildpack.GroupElement{{ID: "D", Version: "v1"}}},
				}

				detector.Order = lifecycle.PrependExtensions(orderBp, orderExt)
				_, _, err := detector.Detect()
				h.AssertNil(t, err)
			})
		})
	})

	when(".Resolve", func() {
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

		it("should fail if the group has no viable buildpacks, even if no required buildpacks fail", func() {
			group := []buildpack.GroupElement{
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
					"fail: no viable buildpacks in group\n",
			) {
				t.Fatalf("Unexpected log:\n%s\n", s)
			}
		})

		when("there are extensions", func() {
			it("should fail if the group has no viable buildpacks, even if no required buildpacks fail", func() {
				group := []buildpack.GroupElement{
					{ID: "A", Version: "v1", Optional: true},
					{ID: "B", Version: "v1", Extension: true, Optional: true},
				}

				detectRuns := &sync.Map{}
				detectRuns.Store("A@v1", buildpack.DetectRun{
					Code: 100,
				})
				detectRuns.Store("B@v1", buildpack.DetectRun{
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

		it("should fail with specific error if any bp detect fails in an unexpected way", func() {
			group := []buildpack.GroupElement{
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
			group := []buildpack.GroupElement{
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
			group := []buildpack.GroupElement{
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
			group := []buildpack.GroupElement{
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

		it("should fail if all requires are not provided first", func() {
			group := []buildpack.GroupElement{
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
			group := []buildpack.GroupElement{
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
			group := []buildpack.GroupElement{
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

		it("should fallback to alternate build plans", func() {
			group := []buildpack.GroupElement{
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
						{OrderExt: expectedOrderExt},
						{ID: "A", Version: "v1"},
					},
				},
				buildpack.Group{
					Group: []buildpack.GroupElement{
						{OrderExt: expectedOrderExt},
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
