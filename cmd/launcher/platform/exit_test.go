package platform_test

import (
	"testing"

	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/cmd/launcher/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExiter(t *testing.T) {
	spec.Run(t, "Test Exiter", testExiter)
}

func testExiter(t *testing.T, when spec.G, it spec.S) {
	when("#NewExiter", func() {
		when("platform api >= 0.6", func() {
			it("returns a default exiter", func() {
				foundExiter := platform.NewExiter("0.6")
				_, ok := foundExiter.(*platform.DefaultExiter)
				h.AssertEq(t, ok, true)
			})
		})

		when("platform api < 0.6", func() {
			it("returns a legacy exiter", func() {
				foundExiter := platform.NewExiter("0.5")
				_, ok := foundExiter.(*platform.LegacyExiter)
				h.AssertEq(t, ok, true)
			})
		})
	})
}
