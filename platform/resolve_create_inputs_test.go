package platform_test

import (
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/path"
	"github.com/buildpacks/lifecycle/internal/str"
	llog "github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestCreateInputs(t *testing.T) {
	for _, api := range api.Platform.Supported {
		spec.Run(t, "unit-create-inputs/"+api.String(), testResolveCreateInputs(api.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testResolveCreateInputs(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var (
			inputs     *platform.LifecycleInputs
			logHandler *memory.Handler
			logger     llog.Logger
		)

		it.Before(func() {
			inputs = platform.NewLifecycleInputs(api.MustParse(platformAPI))
			inputs.OutputImageRef = "some-output-image" // satisfy validation
			inputs.RunImageRef = "some-run-image"       // satisfy validation
			logHandler = memory.New()
			logger = &log.Logger{Handler: logHandler}
			inputs.UseDaemon = true // to prevent read access check for run image
		})

		when("latest Platform API(s)", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.12"), "")
			})

			when("parallel export is enabled and cache image ref is blank", func() {
				it("warns", func() {
					inputs.ParallelExport = true
					inputs.CacheImageRef = ""
					inputs.CacheDir = ""
					err := platform.ResolveInputs(platform.Create, inputs, logger)
					h.AssertNil(t, err)
					expected := "No cached data will be used, no cache specified. Parallel export has been enabled, but it has not taken effect because no cache has been specified."
					h.AssertLogEntry(t, logHandler, expected)
				})
			})

			when("run image", func() {
				when("not provided", func() {
					it.Before(func() {
						inputs.RunImageRef = ""
					})

					it("falls back to run.toml", func() {
						inputs.RunPath = filepath.Join("testdata", "cnb", "run.toml")
						err := platform.ResolveInputs(platform.Create, inputs, logger)
						h.AssertNil(t, err)
						h.AssertEq(t, inputs.RunImageRef, "some-run-image")
					})

					when("run.toml", func() {
						when("not provided", func() {
							it("defaults to /cnb/run.toml", func() {
								_ = platform.ResolveInputs(platform.Create, inputs, logger)
								h.AssertEq(t, inputs.RunPath, filepath.Join(path.RootDir, "cnb", "run.toml"))
							})
						})

						when("not exists", func() {
							it("errors", func() {
								inputs.RunImageRef = ""
								inputs.RunPath = "not-exist-run.toml"
								err := platform.ResolveInputs(platform.Create, inputs, logger)
								h.AssertNotNil(t, err)
								expected := "-run-image is required when there is no run metadata available"
								h.AssertStringContains(t, err.Error(), expected)
							})
						})
					})
				})
			})
		})

		when("Platform API 0.7 to 0.11", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.12"), "")
			})

			when("run image", func() {
				when("not provided", func() {
					it("falls back to stack.toml", func() {
						inputs.RunImageRef = ""
						inputs.StackPath = filepath.Join("testdata", "layers", "stack.toml")
						err := platform.ResolveInputs(platform.Create, inputs, logger)
						h.AssertNil(t, err)
						h.AssertEq(t, inputs.RunImageRef, "some-other-user-provided-run-image")
					})

					when("stack.toml", func() {
						when("not provided", func() {
							it("defaults to /cnb/stack.toml", func() {
								_ = platform.ResolveInputs(platform.Create, inputs, logger)
								h.AssertEq(t, inputs.StackPath, filepath.Join(path.RootDir, "cnb", "stack.toml"))
							})
						})

						when("not exists", func() {
							it("errors", func() {
								inputs.RunImageRef = ""
								inputs.StackPath = "not-exist-stack.toml"
								err := platform.ResolveInputs(platform.Create, inputs, logger)
								h.AssertNotNil(t, err)
								expected := "missing run image metadata (-run-image)"
								h.AssertStringContains(t, err.Error(), expected)
							})
						})
					})
				})
			})
		})

		when("using a registry", func() {
			it.Before(func() {
				inputs.UseDaemon = false
			})

			when("provided destination tags are on different registries", func() {
				it("errors", func() {
					inputs.AdditionalTags = str.Slice{
						"some-registry.io/some-namespace/some-image:tag",
						"some-other-registry.io/some-namespace/some-image",
					}
					inputs.OutputImageRef = "some-registry.io/some-namespace/some-image"
					err := platform.ResolveInputs(platform.Create, inputs, logger)
					h.AssertNotNil(t, err)
					expected := "writing to multiple registries is unsupported"
					h.AssertStringContains(t, err.Error(), expected)
				})
			})
		})
	}
}
