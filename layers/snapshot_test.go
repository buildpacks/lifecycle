package layers_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestSnapshotLayers(t *testing.T) {
	spec.Run(t, "Factory", testSnapshotLayers, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testSnapshotLayers(t *testing.T, when spec.G, it spec.S) {
	it("does something", func() {
		h.AssertEq(t, false, true)
	})
}
