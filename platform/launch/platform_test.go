package launch_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	platform "github.com/buildpacks/lifecycle/platform/launch"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestPlatform(t *testing.T) {
	for _, api := range api.Platform.Supported {
		spec.Run(t, "unit-platform/"+api.String(), testPlatform(api), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testPlatform(platformAPI *api.Version) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		when("#NewPlatform", func() {
			when("platform api >= 0.6", func() {
				it.Before(func() {
					h.SkipIf(t, platformAPI.LessThan("0.6"), "")
				})

				it("configures the platformr", func() {
					foundPlatform := platform.NewPlatform(platformAPI.String())

					t.Log("with a default exiter")
					_, ok := foundPlatform.Exiter.(*platform.DefaultExiter)
					h.AssertEq(t, ok, true)

					t.Log("with an api")
					h.AssertEq(t, foundPlatform.API(), platformAPI)
				})
			})

			when("platform api < 0.6", func() {
				it.Before(func() {
					h.SkipIf(t, platformAPI.AtLeast("0.6"), "")
				})

				it("configures the platform", func() {
					foundPlatform := platform.NewPlatform(platformAPI.String())

					t.Log("with a legacy exiter")
					_, ok := foundPlatform.Exiter.(*platform.LegacyExiter)
					h.AssertEq(t, ok, true)

					t.Log("with an api")
					h.AssertEq(t, foundPlatform.API(), platformAPI)
				})
			})
		})
	}
}
