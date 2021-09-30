package layers_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/layers"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExtract(t *testing.T) {
	spec.Run(t, "Extract", testExtract, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testExtract(t *testing.T, when spec.G, it spec.S) {
	when("#SetUmask", func() {
		it("returns the old umask", func() {
			first := layers.SetUmask(0)
			second := layers.SetUmask(first)
			h.AssertEq(t, second, 0)
		})
	})
}
