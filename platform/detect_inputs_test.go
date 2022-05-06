package platform_test

import (
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDetectInputs(t *testing.T) {
	for _, api := range api.Platform.Supported {
		spec.Run(t, "unit-detector/"+api.String(), testDetectInputs(api.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testDetectInputs(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var (
			resolver   *platform.InputsResolver
			logHandler *memory.Handler
			logger     platform.Logger
		)

		it.Before(func() {
			resolver = platform.NewInputsResolver(api.MustParse(platformAPI))
			logHandler = memory.New()
			logger = &log.Logger{Handler: logHandler}
		})

		when("latest platform api(s)", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.6"), "")
			})

			when("layers directory is provided", func() {
				layersDir := filepath.Join("testdata", "layers")

				when("<layers>/order.toml is present", func() {
					it("uses order.toml at the layers directory and writes group.toml and plan.toml at the layers directory", func() {
						inputs := platform.DetectInputs{
							GroupPath: platform.PlaceholderGroupPath,
							LayersDir: layersDir,
							OrderPath: platform.PlaceholderOrderPath,
							PlanPath:  platform.PlaceholderPlanPath,
						}
						ret, err := resolver.ResolveDetect(inputs, logger)
						h.AssertNil(t, err)
						h.AssertEq(t, ret.OrderPath, filepath.Join(layersDir, "order.toml"))
						h.AssertEq(t, ret.GroupPath, filepath.Join(layersDir, "group.toml"))
						h.AssertEq(t, ret.PlanPath, filepath.Join(layersDir, "plan.toml"))
					})
				})

				when("<layers>/order.toml is not present", func() {
					it("uses /cnb/order.toml and writes group.toml and plan.toml at the layers directory", func() {
						inputs := platform.DetectInputs{
							GroupPath: platform.PlaceholderGroupPath,
							LayersDir: "some-layers-dir",
							OrderPath: platform.PlaceholderOrderPath,
							PlanPath:  platform.PlaceholderPlanPath,
						}
						ret, err := resolver.ResolveDetect(inputs, logger)
						h.AssertNil(t, err)
						h.AssertStringContains(t, ret.OrderPath, filepath.Join("cnb", "order.toml"))
						h.AssertEq(t, ret.GroupPath, filepath.Join("some-layers-dir", "group.toml"))
						h.AssertEq(t, ret.PlanPath, filepath.Join("some-layers-dir", "plan.toml"))
					})
				})
			})
		})

		when("platform api 0.5", func() {
			it.Before(func() {
				h.SkipIf(t, !api.MustParse(platformAPI).Equal(api.MustParse("0.5")), "")
			})

			when("layers directory is provided", func() {
				layersDir := filepath.Join("testdata", "layers")

				it("uses /cnb/order.toml and writes group.toml and plan.toml at the layers directory", func() {
					inputs := platform.DetectInputs{
						GroupPath: platform.PlaceholderGroupPath,
						LayersDir: layersDir,
						OrderPath: platform.PlaceholderOrderPath,
						PlanPath:  platform.PlaceholderPlanPath,
					}
					ret, err := resolver.ResolveDetect(inputs, logger)
					h.AssertNil(t, err)
					h.AssertStringContains(t, ret.OrderPath, filepath.Join("cnb", "order.toml"))
					h.AssertEq(t, ret.GroupPath, filepath.Join(layersDir, "group.toml"))
					h.AssertEq(t, ret.PlanPath, filepath.Join(layersDir, "plan.toml"))
				})
			})
		})

		when("platform api < 0.5", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.5"), "")
			})

			when("layers directory is provided", func() {
				it("uses /cnb/order.toml and writes group.toml and plan.toml at the working directory", func() {
					inputs := platform.DetectInputs{
						GroupPath: platform.PlaceholderGroupPath,
						LayersDir: "some-layers-dir",
						OrderPath: platform.PlaceholderOrderPath,
						PlanPath:  platform.PlaceholderPlanPath}
					ret, err := resolver.ResolveDetect(inputs, logger)
					h.AssertNil(t, err)
					h.AssertStringContains(t, ret.OrderPath, filepath.Join("cnb", "order.toml"))
					h.AssertEq(t, ret.GroupPath, filepath.Join(".", "group.toml"))
					h.AssertEq(t, ret.PlanPath, filepath.Join(".", "plan.toml"))
				})
			})
		})
	}
}
