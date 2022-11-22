package platform_test

import (
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDetectInputs(t *testing.T) {
	for _, platformAPI := range api.Platform.Supported {
		spec.Run(t, "unit-detect-inputs/"+platformAPI.String(), testDetectInputs(platformAPI), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testDetectInputs(platformAPI *api.Version) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		when("Platform API > 0.9", func() {
			it.Before(func() {
				h.SkipIf(t, platformAPI.LessThan("0.10"), "")
			})

			layersDir := filepath.Join("testdata", "layers")

			it("writes analyzed.toml at the layers directory", func() {
				inputs := platform.LifecycleInputs{
					AnalyzedPath: filepath.Join("<layers>", "analyzed.toml"),
					LayersDir:    layersDir,
					PlatformAPI:  platformAPI,
				}
				err := platform.ResolveInputs(platform.Detect, &inputs, nil)
				h.AssertNil(t, err)
				h.AssertEq(t, inputs.AnalyzedPath, filepath.Join(layersDir, "analyzed.toml"))
			})
		})

		when("Platform API > 0.5", func() {
			it.Before(func() {
				h.SkipIf(t, platformAPI.LessThan("0.6"), "")
			})

			layersDir := filepath.Join("testdata", "layers")

			when("<layers>/order.toml is present", func() {
				it("uses order.toml at the layers directory and writes group.toml and plan.toml at the layers directory", func() {
					inputs := platform.LifecycleInputs{
						GroupPath:   filepath.Join("<layers>", "group.toml"),
						LayersDir:   layersDir,
						OrderPath:   filepath.Join("<layers>", "order.toml"),
						PlanPath:    filepath.Join("<layers>", "plan.toml"),
						PlatformAPI: platformAPI,
					}
					err := platform.ResolveInputs(platform.Detect, &inputs, nil)
					h.AssertNil(t, err)
					h.AssertEq(t, inputs.OrderPath, filepath.Join(layersDir, "order.toml"))
					h.AssertEq(t, inputs.GroupPath, filepath.Join(layersDir, "group.toml"))
					h.AssertEq(t, inputs.PlanPath, filepath.Join(layersDir, "plan.toml"))
				})
			})

			when("<layers>/order.toml is not present", func() {
				it("uses /cnb/order.toml and writes group.toml and plan.toml at the layers directory", func() {
					inputs := platform.LifecycleInputs{
						GroupPath:   filepath.Join("<layers>", "group.toml"),
						LayersDir:   "some-layers-dir",
						OrderPath:   filepath.Join("<layers>", "order.toml"),
						PlanPath:    filepath.Join("<layers>", "plan.toml"),
						PlatformAPI: platformAPI,
					}
					err := platform.ResolveInputs(platform.Detect, &inputs, nil)
					h.AssertNil(t, err)
					h.AssertStringContains(t, inputs.OrderPath, filepath.Join("cnb", "order.toml"))
					h.AssertEq(t, inputs.GroupPath, filepath.Join("some-layers-dir", "group.toml"))
					h.AssertEq(t, inputs.PlanPath, filepath.Join("some-layers-dir", "plan.toml"))
				})
			})
		})

		when("Platform API 0.5", func() {
			it.Before(func() {
				h.SkipIf(t, !platformAPI.Equal(api.MustParse("0.5")), "")
			})

			layersDir := filepath.Join("testdata", "layers")

			it("uses /cnb/order.toml and writes group.toml and plan.toml at the layers directory", func() {
				inputs := platform.LifecycleInputs{
					GroupPath:   filepath.Join("<layers>", "group.toml"),
					LayersDir:   layersDir,
					OrderPath:   filepath.Join("<layers>", "order.toml"),
					PlanPath:    filepath.Join("<layers>", "plan.toml"),
					PlatformAPI: platformAPI,
				}
				err := platform.ResolveInputs(platform.Detect, &inputs, nil)
				h.AssertNil(t, err)
				h.AssertStringContains(t, inputs.OrderPath, filepath.Join("cnb", "order.toml"))
				h.AssertEq(t, inputs.GroupPath, filepath.Join(layersDir, "group.toml"))
				h.AssertEq(t, inputs.PlanPath, filepath.Join(layersDir, "plan.toml"))
			})
		})

		when("Platform API < 0.5", func() {
			it.Before(func() {
				h.SkipIf(t, platformAPI.AtLeast("0.5"), "")
			})

			it("uses /cnb/order.toml and writes group.toml and plan.toml at the working directory", func() {
				inputs := platform.LifecycleInputs{
					GroupPath:   filepath.Join("<layers>", "group.toml"),
					LayersDir:   "some-layers-dir",
					OrderPath:   filepath.Join("<layers>", "order.toml"),
					PlanPath:    filepath.Join("<layers>", "plan.toml"),
					PlatformAPI: platformAPI,
				}
				err := platform.ResolveInputs(platform.Detect, &inputs, nil)
				h.AssertNil(t, err)
				h.AssertStringContains(t, inputs.OrderPath, filepath.Join("cnb", "order.toml"))
				h.AssertEq(t, inputs.GroupPath, filepath.Join(".", "group.toml"))
				h.AssertEq(t, inputs.PlanPath, filepath.Join(".", "plan.toml"))
			})
		})
	}
}
