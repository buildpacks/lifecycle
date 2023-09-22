package lifecycle_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/lifecycle"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestHandlers(t *testing.T) {
	spec.Run(t, "Handlers", testHandlers, spec.Report(report.Terminal{}))
}

func testHandlers(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir            string
		groupTOMLContents string
		expectedGroupBp   []buildpack.GroupElement
		expectedGroupExt  []buildpack.GroupElement
		orderTOMLContents string
		expectedOrderBp   buildpack.Order
		expectedOrderExt  buildpack.Order
	)

	it.Before(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "lifecycle.test")
		h.AssertNil(t, err)

		groupTOMLContents = `
[[group]]
id = "A" # intentionally missing version

[[group]]
id = "B"
version = "v1"
homepage = "bp-B-v1-homepage"

[[group-extensions]]
id = "B"
version = "v1"
homepage = "ext-B-v1-homepage"
`
		expectedGroupBp = []buildpack.GroupElement{
			{ID: "A"},
			{ID: "B", Version: "v1", Homepage: "bp-B-v1-homepage"},
		}
		expectedGroupExt = []buildpack.GroupElement{
			{ID: "B", Version: "v1", Homepage: "ext-B-v1-homepage", Extension: true, Optional: true},
		}
		orderTOMLContents = `
[[order]]
group = [{id = "A", version = "v1"}, {id = "B", version = "v1", optional = true}]

[[order]]
group = [{id = "C", version = "v1"}]

[[order-extensions]]
group = [{id = "D", version = "v1"}]
`
		expectedOrderBp = buildpack.Order{
			{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}, {ID: "B", Version: "v1", Optional: true}}},
			{Group: []buildpack.GroupElement{{ID: "C", Version: "v1"}}},
		}
		expectedOrderExt = buildpack.Order{
			{Group: []buildpack.GroupElement{{ID: "D", Version: "v1", Extension: true, Optional: true}}},
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#ReadGroup", func() {
		it("returns a single group object with a buildpack group and an extensions group", func() {
			h.Mkfile(t, groupTOMLContents, filepath.Join(tmpDir, "group.toml"))
			foundGroup, err := lifecycle.ReadGroup(filepath.Join(tmpDir, "group.toml"))
			h.AssertNil(t, err)
			h.AssertEq(t, foundGroup, buildpack.Group{
				Group:           expectedGroupBp,
				GroupExtensions: expectedGroupExt,
			})
		})
	})

	when("#ReadOrder", func() {
		it("returns an ordering of buildpacks and an ordering of extensions", func() {
			h.Mkfile(t, orderTOMLContents, filepath.Join(tmpDir, "order.toml"))
			foundOrder, foundOrderExt, err := lifecycle.ReadOrder(filepath.Join(tmpDir, "order.toml"))
			h.AssertNil(t, err)
			h.AssertEq(t, foundOrder, expectedOrderBp)
			h.AssertEq(t, foundOrderExt, expectedOrderExt)
		})
	})

	when("DefaultConfigHandler", func() {
		var (
			configHandler *lifecycle.DefaultConfigHandler
		)

		it.Before(func() {
			configHandler = lifecycle.NewConfigHandler()
		})

		when(".ReadGroup", func() {
			it("returns a group for buildpacks and a group for extensions", func() {
				h.Mkfile(t, groupTOMLContents, filepath.Join(tmpDir, "group.toml"))
				foundGroup, foundGroupExt, err := configHandler.ReadGroup(filepath.Join(tmpDir, "group.toml"))
				h.AssertNil(t, err)
				h.AssertEq(t, foundGroup, expectedGroupBp)
				h.AssertEq(t, foundGroupExt, expectedGroupExt)
			})
		})

		when(".ReadOrder", func() {
			it("returns an ordering of buildpacks and an ordering of extensions", func() {
				h.Mkfile(t, orderTOMLContents, filepath.Join(tmpDir, "order.toml"))
				foundOrder, foundOrderExt, err := configHandler.ReadOrder(filepath.Join(tmpDir, "order.toml"))
				h.AssertNil(t, err)
				h.AssertEq(t, foundOrder, expectedOrderBp)
				h.AssertEq(t, foundOrderExt, expectedOrderExt)
			})
		})
	})
}
