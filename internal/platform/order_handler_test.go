package platform_test

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/platform"
	"github.com/buildpacks/lifecycle/internal/platform/testmock"
)

func TestOrderHandler(t *testing.T) {
	spec.Run(t, "OrderHandler", testOrderHandler, spec.Report(report.Terminal{}))
}

func testOrderHandler(t *testing.T, when spec.G, it spec.S) {
	when("DefaultOrderHandler", func() {
		var (
			orderHandler   *platform.DefaultOrderHandler
			mockController *gomock.Controller
			execStore      *testmock.MockExecStore // TODO: move mock maybe
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			execStore = testmock.NewMockExecStore(mockController)
			orderHandler = platform.NewDefaultOrderHandler(execStore)
		})

		it.After(func() {
			mockController.Finish()
		})

		when("PrependExtensions", func() {
			it("prepends the extensions order to each group in the buildpacks order", func() {
				orderBp := buildpack.Order{
					buildpack.Group{Group: []buildpack.GroupBuildpack{{ID: "A", Version: "v1"}}},
					buildpack.Group{Group: []buildpack.GroupBuildpack{{ID: "B", Version: "v1"}}},
				}
				orderExt := buildpack.Order{
					buildpack.Group{Group: []buildpack.GroupBuildpack{{ID: "C", Version: "v1"}}},
					buildpack.Group{Group: []buildpack.GroupBuildpack{{ID: "D", Version: "v1"}}},
				}

				t.Log("registers the extensions order with the exec store") // TODO: change to 'detectable'
				execStore.EXPECT().RegisterExtensionsOrder(orderExt)
				newOrder := orderHandler.PrependExtensions(orderBp, orderExt)

				if s := cmp.Diff(newOrder, buildpack.Order{
					buildpack.Group{
						Group: []buildpack.GroupBuildpack{
							{ID: "cnb_order-ext", Version: ""}, // TODO: confirm this approach is sensible
							{ID: "A", Version: "v1"},
						},
					},
					buildpack.Group{
						Group: []buildpack.GroupBuildpack{
							{ID: "cnb_order-ext", Version: ""},
							{ID: "B", Version: "v1"},
						},
					},
				}); s != "" {
					t.Fatalf("Unexpected:\n%s\n", s)
				}
			})
		})
	})

	// TODO
	//when("LegacyOrderHandler", func() {
	//	var orderHandler *platform.LegacyOrderHandler
	//
	//	when("PrependExtensions", func() {
	//
	//	})
	//})
}
