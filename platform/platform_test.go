package platform_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestPlatform(t *testing.T) {
	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "unit-platform/"+platformAPI.String(), testPlatform(platformAPI), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testPlatform(platformAPI *api.Version) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		when("#NewPlatformFor", func() {
			it("configures the platform", func() {
				foundPlatform := platform.NewPlatformFor(platformAPI.String())

				t.Log("with a default exiter")
				_, ok := foundPlatform.Exiter.(*platform.DefaultExiter)
				h.AssertEq(t, ok, true)

				t.Log("with an api")
				h.AssertEq(t, foundPlatform.API(), platformAPI)
			})
		})
	}
}
