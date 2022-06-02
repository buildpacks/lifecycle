package launch_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/launch"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestPlatform(t *testing.T) {
	for _, api := range launch.APIs.Supported {
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

				it("configures the platform", func() {
					foundPlatform := launch.NewPlatform(platformAPI.String())

					t.Log("with a default exiter")
					_, ok := foundPlatform.Exiter.(*launch.DefaultExiter)
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
					foundPlatform := launch.NewPlatform(platformAPI.String())

					t.Log("with a legacy exiter")
					_, ok := foundPlatform.Exiter.(*launch.LegacyExiter)
					h.AssertEq(t, ok, true)

					t.Log("with an api")
					h.AssertEq(t, foundPlatform.API(), platformAPI)
				})
			})
		})

		when("constants", func() {
			it("match platform package", func() {
				h.AssertEq(t, launch.APIs, platform.APIs)
				h.AssertEq(t, launch.EnvAppDir, platform.EnvAppDir)
				h.AssertEq(t, launch.EnvLayersDir, platform.EnvLayersDir)
				h.AssertEq(t, launch.EnvPlatformAPI, platform.EnvPlatformAPI)
				h.AssertEq(t, launch.EnvProcessType, platform.EnvProcessType)
				h.AssertEq(t, launch.CodeForFailed, platform.CodeForFailed)
				h.AssertEq(t, launch.CodeForIncompatiblePlatformAPI, platform.CodeForIncompatiblePlatformAPI)
				h.AssertEq(t, launch.CodeForIncompatibleBuildpackAPI, platform.CodeForIncompatibleBuildpackAPI)
			})
		})
	}
}
