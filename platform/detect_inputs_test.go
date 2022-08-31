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
	for _, api := range api.Platform.Supported {
		spec.Run(t, "unit-detect-inputs/"+api.String(), testDetectInputs(api.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testDetectInputs(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var (
			resolver *platform.InputsResolver
		)

		it.Before(func() {
			resolver = platform.NewInputsResolver(api.MustParse(platformAPI))
		})

		when("directory paths", func() {
			it("resolves absolute paths", func() {
				appDir := filepath.Join("testdata", "workspace")
				appDirAbs, err := filepath.Abs(appDir)
				h.AssertNil(t, err)

				bpDir := filepath.Join("testdata", "cnb", "buildpacks")
				bpDirAbs, err := filepath.Abs(bpDir)
				h.AssertNil(t, err)

				extDir := filepath.Join("testdata", "cnb", "extensions")
				extDirAbs, err := filepath.Abs(extDir)
				h.AssertNil(t, err)

				layersDir := filepath.Join("testdata", "layers")
				layersDirAbs, err := filepath.Abs(layersDir)
				h.AssertNil(t, err)

				generatedDir := filepath.Join("testdata", "layers")
				generatedDirAbs, err := filepath.Abs(generatedDir)
				h.AssertNil(t, err)

				platformDir := filepath.Join("testdata", "platform")
				platformDirAbs, err := filepath.Abs(platformDir)
				h.AssertNil(t, err)

				inputs := platform.DetectInputs{
					AppDir:        appDir,
					BuildpacksDir: bpDir,
					ExtensionsDir: extDir,
					GeneratedDir:  generatedDir,
					LayersDir:     layersDir,
					PlatformDir:   platformDir,
				}
				ret, err := resolver.ResolveDetect(inputs)
				h.AssertNil(t, err)
				h.AssertEq(t, ret.AppDir, appDirAbs)
				h.AssertEq(t, ret.BuildpacksDir, bpDirAbs)
				h.AssertEq(t, ret.ExtensionsDir, extDirAbs)
				h.AssertEq(t, ret.LayersDir, layersDirAbs)
				h.AssertEq(t, ret.GeneratedDir, generatedDirAbs)
				h.AssertEq(t, ret.PlatformDir, platformDirAbs)
			})

			when("paths are empty", func() {
				it("resolves to an empty string", func() {
					inputs := platform.DetectInputs{}
					ret, err := resolver.ResolveDetect(inputs)
					h.AssertNil(t, err)
					h.AssertEq(t, ret.AppDir, "")
					h.AssertEq(t, ret.BuildpacksDir, "")
					h.AssertEq(t, ret.ExtensionsDir, "")
					h.AssertEq(t, ret.LayersDir, "")
					h.AssertEq(t, ret.GeneratedDir, "")
					h.AssertEq(t, ret.PlatformDir, "")
				})
			})
		})

		when("platform api > 0.9", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.10"), "")
			})

			layersDir := filepath.Join("testdata", "layers")

			it("writes analyzed.toml at the layers directory", func() {
				inputs := platform.DetectInputs{
					AnalyzedPath: platform.PlaceholderAnalyzedPath,
					LayersDir:    layersDir,
				}
				ret, err := resolver.ResolveDetect(inputs)
				h.AssertNil(t, err)
				h.AssertEq(t, ret.AnalyzedPath, filepath.Join(layersDir, "analyzed.toml"))
			})
		})

		when("platform api > 0.5", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.6"), "")
			})

			layersDir := filepath.Join("testdata", "layers")

			when("<layers>/order.toml is present", func() {
				it("uses order.toml at the layers directory and writes group.toml and plan.toml at the layers directory", func() {
					inputs := platform.DetectInputs{
						GroupPath: platform.PlaceholderGroupPath,
						LayersDir: layersDir,
						OrderPath: platform.PlaceholderOrderPath,
						PlanPath:  platform.PlaceholderPlanPath,
					}
					ret, err := resolver.ResolveDetect(inputs)
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
					ret, err := resolver.ResolveDetect(inputs)
					h.AssertNil(t, err)
					h.AssertStringContains(t, ret.OrderPath, filepath.Join("cnb", "order.toml"))
					h.AssertEq(t, ret.GroupPath, filepath.Join("some-layers-dir", "group.toml"))
					h.AssertEq(t, ret.PlanPath, filepath.Join("some-layers-dir", "plan.toml"))
				})
			})
		})

		when("platform api 0.5", func() {
			it.Before(func() {
				h.SkipIf(t, !api.MustParse(platformAPI).Equal(api.MustParse("0.5")), "")
			})

			layersDir := filepath.Join("testdata", "layers")

			it("uses /cnb/order.toml and writes group.toml and plan.toml at the layers directory", func() {
				inputs := platform.DetectInputs{
					GroupPath: platform.PlaceholderGroupPath,
					LayersDir: layersDir,
					OrderPath: platform.PlaceholderOrderPath,
					PlanPath:  platform.PlaceholderPlanPath,
				}
				ret, err := resolver.ResolveDetect(inputs)
				h.AssertNil(t, err)
				h.AssertStringContains(t, ret.OrderPath, filepath.Join("cnb", "order.toml"))
				h.AssertEq(t, ret.GroupPath, filepath.Join(layersDir, "group.toml"))
				h.AssertEq(t, ret.PlanPath, filepath.Join(layersDir, "plan.toml"))
			})
		})

		when("platform api < 0.5", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.5"), "")
			})

			it("uses /cnb/order.toml and writes group.toml and plan.toml at the working directory", func() {
				inputs := platform.DetectInputs{
					GroupPath: platform.PlaceholderGroupPath,
					LayersDir: "some-layers-dir",
					OrderPath: platform.PlaceholderOrderPath,
					PlanPath:  platform.PlaceholderPlanPath}
				ret, err := resolver.ResolveDetect(inputs)
				h.AssertNil(t, err)
				h.AssertStringContains(t, ret.OrderPath, filepath.Join("cnb", "order.toml"))
				h.AssertEq(t, ret.GroupPath, filepath.Join(".", "group.toml"))
				h.AssertEq(t, ret.PlanPath, filepath.Join(".", "plan.toml"))
			})
		})
	}
}
