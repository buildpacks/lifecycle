package lifecycle_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/lifecycle"
)

func TestOrderHandler(t *testing.T) {
	spec.Run(t, "OrderHandler", testOrderHandler, spec.Report(report.Terminal{}))
}

func testOrderHandler(t *testing.T, when spec.G, it spec.S) {
	when("DefaultOrderHandler", func() {
		var orderHandler = &lifecycle.DefaultOrderHandler{}

		when("PrependExtensions", func() {
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

				orderHandler.PrependExtensions(orderBp, orderExt)

				if s := cmp.Diff(orderBp, buildpack.Order{
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
			})

			when("the extensions order is empty", func() {
				it("does not modify the buildpacks order", func() {
					orderBp := buildpack.Order{
						buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
						buildpack.Group{Group: []buildpack.GroupElement{{ID: "B", Version: "v1"}}},
					}

					orderHandler.PrependExtensions(orderBp, nil)

					if s := cmp.Diff(orderBp, buildpack.Order{
						buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
						buildpack.Group{Group: []buildpack.GroupElement{{ID: "B", Version: "v1"}}},
					}); s != "" {
						t.Fatalf("Unexpected:\n%s\n", s)
					}
				})
			})
		})
	})

	when("LegacyOrderHandler", func() {
		var orderHandler = &lifecycle.LegacyOrderHandler{}

		when("PrependExtensions", func() {
			it("does not modify the buildpacks order", func() {
				orderBp := buildpack.Order{
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "B", Version: "v1"}}},
				}
				orderExt := buildpack.Order{
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "C", Version: "v1"}}},
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "D", Version: "v1"}}},
				}

				orderHandler.PrependExtensions(orderBp, orderExt)

				if s := cmp.Diff(orderBp, buildpack.Order{
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}},
					buildpack.Group{Group: []buildpack.GroupElement{{ID: "B", Version: "v1"}}},
				}); s != "" {
					t.Fatalf("Unexpected:\n%s\n", s)
				}
			})
		})
	})
}
