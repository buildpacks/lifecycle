package platform_test

import (
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/str"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestAnalyzeInputs(t *testing.T) {
	for _, api := range platform.APIs.Supported {
		spec.Run(t, "unit-analyzer/"+api.String(), testAnalyzeInputs(api.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testAnalyzeInputs(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
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
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.7"), "")
			})

			when("run image", func() {
				when("not provided", func() {
					it("falls back to stack.toml", func() {
						inputs := platform.AnalyzeInputs{
							StackPath:      filepath.Join("testdata", "layers", "stack.toml"),
							OutputImageRef: "some-image",
						}
						ret, err := resolver.ResolveAnalyze(inputs, logger)
						h.AssertNil(t, err)
						h.AssertEq(t, ret.RunImageRef, "some-run-image")
					})

					when("stack.toml not present", func() {
						it("errors", func() {
							inputs := platform.AnalyzeInputs{
								StackPath:      "not-exist-stack.toml",
								OutputImageRef: "some-image",
							}
							_, err := resolver.ResolveAnalyze(inputs, logger)
							h.AssertNotNil(t, err)
							expected := "-run-image is required when there is no stack metadata available"
							h.AssertStringContains(t, err.Error(), expected)
						})
					})
				})
			})

			when("provided destination tags are on different registries", func() {
				it("errors", func() {
					inputs := platform.AnalyzeInputs{
						AdditionalTags: str.Slice{
							"some-registry.io/some-namespace/some-image:tag",
							"some-other-registry.io/some-namespace/some-image",
						},
						OutputImageRef: "some-registry.io/some-namespace/some-image",
						RunImageRef:    "some-run-image-ref", // ignore
					}
					_, err := resolver.ResolveAnalyze(inputs, logger)
					h.AssertNotNil(t, err)
					expected := "writing to multiple registries is unsupported"
					h.AssertStringContains(t, err.Error(), expected)
				})
			})
		})

		when("platform api < 0.7", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "")
			})

			when("cache image tag and cache directory are both blank", func() {
				it("warns", func() {
					inputs := platform.AnalyzeInputs{
						OutputImageRef: "some-image",
					}
					_, err := resolver.ResolveAnalyze(inputs, logger)
					h.AssertNil(t, err)
					expected := "Not restoring cached layer metadata, no cache flag specified."
					h.AssertLogEntry(t, logHandler, expected)
				})
			})

			when("run image", func() {
				when("not provided", func() {
					it("does not warn", func() {
						inputs := platform.AnalyzeInputs{
							StackPath:      "not-exist-stack.toml",
							OutputImageRef: "some-image",
						}
						_, err := resolver.ResolveAnalyze(inputs, logger)
						h.AssertNil(t, err)
						h.AssertNoLogEntry(t, logHandler, `no stack metadata found at path ''`)
						h.AssertNoLogEntry(t, logHandler, `Previous image with name "" not found`)
					})
				})
			})

			when("layers path is provided", func() {
				it("uses the group path at the layers path and writes analyzed.toml at the layers path", func() {
					h.SkipIf(t,
						api.MustParse(platformAPI).LessThan("0.5"),
						"Platform API < 0.5 reads and writes to the working directory",
					)

					inputs := platform.AnalyzeInputs{
						AnalyzedPath:    platform.PlaceholderAnalyzedPath,
						LegacyGroupPath: platform.PlaceholderGroupPath,
						LayersDir:       "some-layers-dir",
						OutputImageRef:  "some-image",
					}
					ret, err := resolver.ResolveAnalyze(inputs, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, ret.LegacyGroupPath, filepath.Join("some-layers-dir", "group.toml"))
					h.AssertEq(t, ret.AnalyzedPath, filepath.Join("some-layers-dir", "analyzed.toml"))
				})
			})
		})

		when("platform api < 0.5", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.6"), "")
			})

			when("layers path is provided", func() {
				it("uses the group path at the working directory and writes analyzed.toml at the working directory", func() {
					inputs := platform.AnalyzeInputs{
						AnalyzedPath:    filepath.Join(".", "analyzed.toml"),
						LegacyGroupPath: filepath.Join(".", "group.toml"),
						LayersDir:       filepath.Join("testdata", "other-layers"),
						OutputImageRef:  "some-image",
					}
					ret, err := resolver.ResolveAnalyze(inputs, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, ret.LegacyGroupPath, filepath.Join(".", "group.toml"))
					h.AssertEq(t, ret.AnalyzedPath, filepath.Join(".", "analyzed.toml"))
				})
			})
		})
	}
}
