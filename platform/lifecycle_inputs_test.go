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
			h.AssertEq(t, inputs.InsecureRegistries, str.Slice(nil))
		})

		when("env vars are set", func() {
			it.Before(func() {
				h.AssertNil(t, os.Setenv(platform.EnvAnalyzedPath, "some-analyzed-path"))
				h.AssertNil(t, os.Setenv(platform.EnvAppDir, "some-app-dir"))
				h.AssertNil(t, os.Setenv(platform.EnvBuildConfigDir, "some-build-config-dir"))
				h.AssertNil(t, os.Setenv(platform.EnvBuildImage, "some-build-image"))
				h.AssertNil(t, os.Setenv(platform.EnvBuildpacksDir, "some-buildpacks-dir"))
				h.AssertNil(t, os.Setenv(platform.EnvCacheDir, "some-cache-dir"))
				h.AssertNil(t, os.Setenv(platform.EnvCacheImage, "some-cache-image"))
				h.AssertNil(t, os.Setenv(platform.EnvExtendKind, "run"))
				h.AssertNil(t, os.Setenv(platform.EnvExtensionsDir, "some-extensions-dir"))
				h.AssertNil(t, os.Setenv(platform.EnvGID, "5678"))
				h.AssertNil(t, os.Setenv(platform.EnvForceRebase, "true"))
				h.AssertNil(t, os.Setenv(platform.EnvGeneratedDir, "some-generated-dir"))
				h.AssertNil(t, os.Setenv(platform.EnvGroupPath, "some-group-path"))
				h.AssertNil(t, os.Setenv(platform.EnvKanikoCacheTTL, "1h0m0s"))
				h.AssertNil(t, os.Setenv(platform.EnvLaunchCacheDir, "some-launch-cache-dir"))
				h.AssertNil(t, os.Setenv(platform.EnvLayersDir, "some-layers-dir"))
				h.AssertNil(t, os.Setenv(platform.EnvLayoutDir, "some-layout-dir"))
				h.AssertNil(t, os.Setenv(platform.EnvLogLevel, "debug"))
				h.AssertNil(t, os.Setenv(platform.EnvOrderPath, "some-order-path"))
				h.AssertNil(t, os.Setenv(platform.EnvPlanPath, "some-plan-path"))
				h.AssertNil(t, os.Setenv(platform.EnvPlatformDir, "some-platform-dir"))
				h.AssertNil(t, os.Setenv(platform.EnvPreviousImage, "some-previous-image"))
				h.AssertNil(t, os.Setenv(platform.EnvProcessType, "some-process-type"))
				h.AssertNil(t, os.Setenv(platform.EnvReportPath, "some-report-path"))
				h.AssertNil(t, os.Setenv(platform.EnvRunImage, "some-run-image"))
				h.AssertNil(t, os.Setenv(platform.EnvRunPath, "some-run-path"))
				h.AssertNil(t, os.Setenv(platform.EnvSkipLayers, "true"))
				h.AssertNil(t, os.Setenv(platform.EnvStackPath, "some-stack-path"))
				h.AssertNil(t, os.Setenv(platform.EnvUID, "1234"))
				h.AssertNil(t, os.Setenv(platform.EnvUseDaemon, "true"))
				h.AssertNil(t, os.Setenv(platform.EnvUseLayout, "true"))
				h.AssertNil(t, os.Setenv(platform.EnvInsecureRegistry, "some-insecure-registry,another-insecure-registry, just-another-registry"))
			})

			it.After(func() {
				h.AssertNil(t, os.Unsetenv(platform.EnvAnalyzedPath))
				h.AssertNil(t, os.Unsetenv(platform.EnvAppDir))
				h.AssertNil(t, os.Unsetenv(platform.EnvBuildConfigDir))
				h.AssertNil(t, os.Unsetenv(platform.EnvBuildImage))
				h.AssertNil(t, os.Unsetenv(platform.EnvBuildpacksDir))
				h.AssertNil(t, os.Unsetenv(platform.EnvCacheDir))
				h.AssertNil(t, os.Unsetenv(platform.EnvCacheImage))
				h.AssertNil(t, os.Unsetenv(platform.EnvExtendKind))
				h.AssertNil(t, os.Unsetenv(platform.EnvExtensionsDir))
				h.AssertNil(t, os.Unsetenv(platform.EnvForceRebase))
				h.AssertNil(t, os.Unsetenv(platform.EnvGID))
				h.AssertNil(t, os.Unsetenv(platform.EnvGeneratedDir))
				h.AssertNil(t, os.Unsetenv(platform.EnvGroupPath))
				h.AssertNil(t, os.Unsetenv(platform.EnvKanikoCacheTTL))
				h.AssertNil(t, os.Unsetenv(platform.EnvLaunchCacheDir))
				h.AssertNil(t, os.Unsetenv(platform.EnvLayersDir))
				h.AssertNil(t, os.Unsetenv(platform.EnvLayoutDir))
				h.AssertNil(t, os.Unsetenv(platform.EnvLogLevel))
				h.AssertNil(t, os.Unsetenv(platform.EnvOrderPath))
				h.AssertNil(t, os.Unsetenv(platform.EnvPlanPath))
				h.AssertNil(t, os.Unsetenv(platform.EnvPlatformDir))
				h.AssertNil(t, os.Unsetenv(platform.EnvPreviousImage))
				h.AssertNil(t, os.Unsetenv(platform.EnvProcessType))
				h.AssertNil(t, os.Unsetenv(platform.EnvReportPath))
				h.AssertNil(t, os.Unsetenv(platform.EnvRunImage))
				h.AssertNil(t, os.Unsetenv(platform.EnvRunPath))
				h.AssertNil(t, os.Unsetenv(platform.EnvSkipLayers))
				h.AssertNil(t, os.Unsetenv(platform.EnvStackPath))
				h.AssertNil(t, os.Unsetenv(platform.EnvUID))
				h.AssertNil(t, os.Unsetenv(platform.EnvUseDaemon))
				h.AssertNil(t, os.Unsetenv(platform.EnvUseLayout))
				h.AssertNil(t, os.Unsetenv(platform.EnvInsecureRegistry))
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
				h.AssertEq(t, inputs.InsecureRegistries, str.Slice{
					"some-insecure-registry",
					"another-insecure-registry",
					"just-another-registry",
				})
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
