package platform_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/str"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/env"
	h "github.com/buildpacks/lifecycle/testhelpers"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestLifecycleInputs(t *testing.T) {
	spec.Run(t, "LifecycleInputs", testLifecycleInputs, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testLifecycleInputs(t *testing.T, when spec.G, it spec.S) {
	when("#NewLifecycleInputs", func() {
		var (
			platformAPI = api.Platform.Latest()
			inputs      *platform.LifecycleInputs
		)

		it("returns lifecycle inputs with default values fill in", func() {
			inputs = platform.NewLifecycleInputs(platformAPI)

			h.AssertEq(t, inputs.AdditionalTags, str.Slice(nil))
			h.AssertEq(t, inputs.AppDir, platform.DefaultAppDir)
			h.AssertEq(t, inputs.BuildConfigDir, platform.DefaultBuildConfigDir)
			h.AssertEq(t, inputs.BuildImageRef, "")
			h.AssertEq(t, inputs.BuildpacksDir, platform.DefaultBuildpacksDir)
			h.AssertEq(t, inputs.CacheDir, "")
			h.AssertEq(t, inputs.CacheImageRef, "")
			h.AssertEq(t, inputs.DefaultProcessType, "")
			h.AssertEq(t, inputs.DeprecatedRunImageRef, "")
			h.AssertEq(t, inputs.ExtendKind, "build")
			h.AssertEq(t, inputs.ExtensionsDir, platform.DefaultExtensionsDir)
			h.AssertEq(t, inputs.ForceRebase, false)
			h.AssertEq(t, inputs.GID, 0)
			h.AssertEq(t, inputs.KanikoCacheTTL, platform.DefaultKanikoCacheTTL)
			h.AssertEq(t, inputs.KanikoDir, "/kaniko")
			h.AssertEq(t, inputs.LaunchCacheDir, "")
			h.AssertEq(t, inputs.LauncherPath, platform.DefaultLauncherPath)
			h.AssertEq(t, inputs.LauncherSBOMDir, platform.DefaultBuildpacksioSBOMDir)
			h.AssertEq(t, inputs.LayersDir, platform.DefaultLayersDir)
			h.AssertEq(t, inputs.LogLevel, "info")
			h.AssertEq(t, inputs.OutputImageRef, "")
			h.AssertEq(t, inputs.PlatformAPI, platformAPI) // from constructor
			h.AssertEq(t, inputs.PlatformDir, platform.DefaultPlatformDir)
			h.AssertEq(t, inputs.PreviousImageRef, "")
			h.AssertEq(t, inputs.RunImageRef, "")
			h.AssertEq(t, inputs.RunPath, platform.DefaultRunPath)
			h.AssertEq(t, inputs.SkipLayers, false)
			h.AssertEq(t, inputs.StackPath, platform.DefaultStackPath)
			h.AssertEq(t, inputs.UID, 0)
			h.AssertEq(t, inputs.UseDaemon, false)
			h.AssertEq(t, inputs.UseLayout, false)
		})

		when("env vars are set", func() {
			it.Before(func() {
				h.AssertNil(t, os.Setenv(env.VarAnalyzedPath, "some-analyzed-path"))
				h.AssertNil(t, os.Setenv(env.VarAppDir, "some-app-dir"))
				h.AssertNil(t, os.Setenv(env.VarBuildConfigDir, "some-build-config-dir"))
				h.AssertNil(t, os.Setenv(env.VarBuildImage, "some-build-image"))
				h.AssertNil(t, os.Setenv(env.VarBuildpacksDir, "some-buildpacks-dir"))
				h.AssertNil(t, os.Setenv(env.VarCacheDir, "some-cache-dir"))
				h.AssertNil(t, os.Setenv(env.VarCacheImage, "some-cache-image"))
				h.AssertNil(t, os.Setenv(env.VarExtendKind, "run"))
				h.AssertNil(t, os.Setenv(env.VarExtensionsDir, "some-extensions-dir"))
				h.AssertNil(t, os.Setenv(env.VarGID, "5678"))
				h.AssertNil(t, os.Setenv(env.VarForceRebase, "true"))
				h.AssertNil(t, os.Setenv(env.VarGeneratedDir, "some-generated-dir"))
				h.AssertNil(t, os.Setenv(env.VarGroupPath, "some-group-path"))
				h.AssertNil(t, os.Setenv(env.VarKanikoCacheTTL, "1h0m0s"))
				h.AssertNil(t, os.Setenv(env.VarLaunchCacheDir, "some-launch-cache-dir"))
				h.AssertNil(t, os.Setenv(env.VarLayersDir, "some-layers-dir"))
				h.AssertNil(t, os.Setenv(env.VarLayoutDir, "some-layout-dir"))
				h.AssertNil(t, os.Setenv(env.VarLogLevel, "debug"))
				h.AssertNil(t, os.Setenv(env.VarOrderPath, "some-order-path"))
				h.AssertNil(t, os.Setenv(env.VarPlanPath, "some-plan-path"))
				h.AssertNil(t, os.Setenv(env.VarPlatformDir, "some-platform-dir"))
				h.AssertNil(t, os.Setenv(env.VarPreviousImage, "some-previous-image"))
				h.AssertNil(t, os.Setenv(env.VarProcessType, "some-process-type"))
				h.AssertNil(t, os.Setenv(env.VarReportPath, "some-report-path"))
				h.AssertNil(t, os.Setenv(env.VarRunImage, "some-run-image"))
				h.AssertNil(t, os.Setenv(env.VarRunPath, "some-run-path"))
				h.AssertNil(t, os.Setenv(env.VarSkipLayers, "true"))
				h.AssertNil(t, os.Setenv(env.VarStackPath, "some-stack-path"))
				h.AssertNil(t, os.Setenv(env.VarUID, "1234"))
				h.AssertNil(t, os.Setenv(env.VarUseDaemon, "true"))
				h.AssertNil(t, os.Setenv(env.VarUseLayout, "true"))
			})

			it.After(func() {
				h.AssertNil(t, os.Unsetenv(env.VarAnalyzedPath))
				h.AssertNil(t, os.Unsetenv(env.VarAppDir))
				h.AssertNil(t, os.Unsetenv(env.VarBuildConfigDir))
				h.AssertNil(t, os.Unsetenv(env.VarBuildImage))
				h.AssertNil(t, os.Unsetenv(env.VarBuildpacksDir))
				h.AssertNil(t, os.Unsetenv(env.VarCacheDir))
				h.AssertNil(t, os.Unsetenv(env.VarCacheImage))
				h.AssertNil(t, os.Unsetenv(env.VarExtendKind))
				h.AssertNil(t, os.Unsetenv(env.VarExtensionsDir))
				h.AssertNil(t, os.Unsetenv(env.VarForceRebase))
				h.AssertNil(t, os.Unsetenv(env.VarGID))
				h.AssertNil(t, os.Unsetenv(env.VarGeneratedDir))
				h.AssertNil(t, os.Unsetenv(env.VarGroupPath))
				h.AssertNil(t, os.Unsetenv(env.VarKanikoCacheTTL))
				h.AssertNil(t, os.Unsetenv(env.VarLaunchCacheDir))
				h.AssertNil(t, os.Unsetenv(env.VarLayersDir))
				h.AssertNil(t, os.Unsetenv(env.VarLayoutDir))
				h.AssertNil(t, os.Unsetenv(env.VarLogLevel))
				h.AssertNil(t, os.Unsetenv(env.VarOrderPath))
				h.AssertNil(t, os.Unsetenv(env.VarPlanPath))
				h.AssertNil(t, os.Unsetenv(env.VarPlatformDir))
				h.AssertNil(t, os.Unsetenv(env.VarPreviousImage))
				h.AssertNil(t, os.Unsetenv(env.VarProcessType))
				h.AssertNil(t, os.Unsetenv(env.VarReportPath))
				h.AssertNil(t, os.Unsetenv(env.VarRunImage))
				h.AssertNil(t, os.Unsetenv(env.VarRunPath))
				h.AssertNil(t, os.Unsetenv(env.VarSkipLayers))
				h.AssertNil(t, os.Unsetenv(env.VarStackPath))
				h.AssertNil(t, os.Unsetenv(env.VarUID))
				h.AssertNil(t, os.Unsetenv(env.VarUseDaemon))
				h.AssertNil(t, os.Unsetenv(env.VarUseLayout))
			})

			it("returns lifecycle inputs with env values fill in", func() {
				inputs = platform.NewLifecycleInputs(platformAPI)

				h.AssertEq(t, inputs.AdditionalTags, str.Slice(nil))
				h.AssertEq(t, inputs.AnalyzedPath, "some-analyzed-path")
				h.AssertEq(t, inputs.AppDir, "some-app-dir")
				h.AssertEq(t, inputs.BuildConfigDir, "some-build-config-dir")
				h.AssertEq(t, inputs.BuildImageRef, "some-build-image")
				h.AssertEq(t, inputs.BuildpacksDir, "some-buildpacks-dir")
				h.AssertEq(t, inputs.CacheDir, "some-cache-dir")
				h.AssertEq(t, inputs.CacheImageRef, "some-cache-image")
				h.AssertEq(t, inputs.DefaultProcessType, "some-process-type")
				h.AssertEq(t, inputs.DeprecatedRunImageRef, "")
				h.AssertEq(t, inputs.ExtendKind, "run")
				h.AssertEq(t, inputs.ExtensionsDir, "some-extensions-dir")
				h.AssertEq(t, inputs.ForceRebase, true)
				h.AssertEq(t, inputs.GID, 5678)
				h.AssertEq(t, inputs.GeneratedDir, "some-generated-dir")
				h.AssertEq(t, inputs.GroupPath, "some-group-path")
				h.AssertEq(t, inputs.KanikoCacheTTL, 1*time.Hour)
				h.AssertEq(t, inputs.LaunchCacheDir, "some-launch-cache-dir")
				h.AssertEq(t, inputs.LauncherPath, platform.DefaultLauncherPath)
				h.AssertEq(t, inputs.LauncherSBOMDir, platform.DefaultBuildpacksioSBOMDir)
				h.AssertEq(t, inputs.LayersDir, "some-layers-dir")
				h.AssertEq(t, inputs.LayoutDir, "some-layout-dir")
				h.AssertEq(t, inputs.LogLevel, "debug")
				h.AssertEq(t, inputs.OrderPath, "some-order-path")
				h.AssertEq(t, inputs.OutputImageRef, "")
				h.AssertEq(t, inputs.PlanPath, "some-plan-path")
				h.AssertEq(t, inputs.PlatformAPI, platformAPI) // from constructor
				h.AssertEq(t, inputs.PlatformDir, "some-platform-dir")
				h.AssertEq(t, inputs.PreviousImageRef, "some-previous-image")
				h.AssertEq(t, inputs.ReportPath, "some-report-path")
				h.AssertEq(t, inputs.RunImageRef, "some-run-image")
				h.AssertEq(t, inputs.RunPath, "some-run-path")
				h.AssertEq(t, inputs.SkipLayers, true)
				h.AssertEq(t, inputs.StackPath, "some-stack-path")
				h.AssertEq(t, inputs.UID, 1234)
				h.AssertEq(t, inputs.UseDaemon, true)
				h.AssertEq(t, inputs.UseLayout, true)
			})
		})

		it("expects and writes files in the layers directory", func() {
			inputs = platform.NewLifecycleInputs(platformAPI)

			h.AssertEq(t, inputs.AnalyzedPath, filepath.Join("<layers>", "analyzed.toml"))
			h.AssertEq(t, inputs.GeneratedDir, filepath.Join("<layers>", "generated"))
			h.AssertEq(t, inputs.GroupPath, filepath.Join("<layers>", "group.toml"))
			h.AssertEq(t, inputs.OrderPath, filepath.Join("<layers>", "order.toml"))
			h.AssertEq(t, inputs.PlanPath, filepath.Join("<layers>", "plan.toml"))
			h.AssertEq(t, inputs.ProjectMetadataPath, filepath.Join("<layers>", "project-metadata.toml"))
			h.AssertEq(t, inputs.ReportPath, filepath.Join("<layers>", "report.toml"))
		})

		when("Platform API = 0.5", func() {
			platformAPI = api.MustParse("0.5")

			it("expects and writes files in the layers directory", func() {
				inputs = platform.NewLifecycleInputs(platformAPI)

				h.AssertEq(t, inputs.AnalyzedPath, filepath.Join("<layers>", "analyzed.toml"))
				h.AssertEq(t, inputs.GroupPath, filepath.Join("<layers>", "group.toml"))
				h.AssertEq(t, inputs.PlanPath, filepath.Join("<layers>", "plan.toml"))
				h.AssertEq(t, inputs.ProjectMetadataPath, filepath.Join("<layers>", "project-metadata.toml"))
				h.AssertEq(t, inputs.ReportPath, filepath.Join("<layers>", "report.toml"))
			})

			it("expects order.toml in the /cnb directory", func() {
				inputs = platform.NewLifecycleInputs(platformAPI)

				h.AssertEq(t, inputs.OrderPath, platform.CNBOrderPath)
			})
		})

		when("Platform API < 0.5", func() {
			platformAPI = api.MustParse("0.4")

			it("expects and writes files in the working directory", func() {
				inputs = platform.NewLifecycleInputs(platformAPI)

				h.AssertEq(t, inputs.AnalyzedPath, "analyzed.toml")
				h.AssertEq(t, inputs.GroupPath, "group.toml")
				h.AssertEq(t, inputs.PlanPath, "plan.toml")
				h.AssertEq(t, inputs.ProjectMetadataPath, "project-metadata.toml")
				h.AssertEq(t, inputs.ReportPath, "report.toml")
			})

			it("expects order.toml in the /cnb directory", func() {
				inputs = platform.NewLifecycleInputs(platformAPI)
				h.AssertEq(t, inputs.OrderPath, platform.CNBOrderPath)
			})
		})
	})

	when("#UpdatePlaceholderPaths", func() {
		var (
			platformAPI = api.Platform.Latest()
			inputs      *platform.LifecycleInputs
		)

		it.Before(func() {
			inputs = platform.NewLifecycleInputs(platformAPI)
		})

		it("updates all placeholder paths", func() {
			h.AssertNil(t, platform.UpdatePlaceholderPaths(inputs, nil))
			v := reflect.ValueOf(inputs).Elem()
			for i := 0; i < v.NumField(); i++ {
				field := v.Field(i)
				if !(field.Kind() == reflect.String) {
					continue
				}
				h.AssertStringDoesNotContain(t, field.String(), platform.PlaceholderLayers)
			}
		})

		when("path is blank", func() {
			it.Before(func() {
				inputs.AnalyzedPath = ""
			})

			it("does nothing", func() {
				h.AssertNil(t, platform.UpdatePlaceholderPaths(inputs, nil))
				h.AssertEq(t, inputs.AnalyzedPath, "")
			})
		})

		when("order.toml", func() {
			when("custom", func() {
				it("doesn't override it", func() {
					inputs.OrderPath = "some-order-path"
					h.AssertNil(t, platform.UpdatePlaceholderPaths(inputs, nil))
					h.AssertEq(t, inputs.OrderPath, inputs.OrderPath)
				})
			})

			when("exists in layers directory", func() {
				var tmpDir string

				it.Before(func() {
					var err error
					tmpDir, err = os.MkdirTemp("", "lifecycle")
					h.AssertNil(t, err)
					h.Mkfile(t, "", filepath.Join(tmpDir, "order.toml"))
					inputs.LayersDir = tmpDir
				})

				it.After(func() {
					_ = os.RemoveAll(tmpDir)
				})

				it("expects order.toml in the layers directory", func() {
					h.AssertNil(t, platform.UpdatePlaceholderPaths(inputs, nil))
					h.AssertEq(t, inputs.OrderPath, filepath.Join(tmpDir, "order.toml"))
				})
			})

			when("not exists in layers directory", func() {
				it("expects order.toml in the /cnb directory", func() {
					h.AssertNil(t, platform.UpdatePlaceholderPaths(inputs, nil))
					h.AssertEq(t, inputs.OrderPath, platform.CNBOrderPath)
				})
			})
		})

		when("layers is blank", func() {
			it.Before(func() {
				inputs.LayersDir = ""
			})

			it("expects and writes files in the working directory", func() {
				h.AssertNil(t, platform.UpdatePlaceholderPaths(inputs, nil))
				h.AssertEq(t, inputs.AnalyzedPath, "analyzed.toml")
			})
		})
	})

	when("#ValidateSameRegistry", func() {
		when("multiple registries are provided", func() {
			it("errors as unsupported", func() {
				err := platform.ValidateSameRegistry("some/repo", "gcr.io/other-repo:latest", "example.com/final-repo")
				h.AssertError(t, err, "writing to multiple registries is unsupported")
			})
		})

		when("a single registry is provided", func() {
			it("does not return an error", func() {
				err := platform.ValidateSameRegistry("gcr.io/some/repo", "gcr.io/other-repo:latest", "gcr.io/final-repo")
				h.AssertNil(t, err)
			})
		})

		when("the tag reference is invalid", func() {
			it("errors", func() {
				err := platform.ValidateSameRegistry("some/Repo")
				h.AssertError(t, err, "could not parse reference: some/Repo")
			})
		})
	})
}
