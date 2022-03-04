package platform_test

import (
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cmd/lifecycle/platform"
	"github.com/buildpacks/lifecycle/internal/str"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestAnalyzeInputs(t *testing.T) {
	for _, api := range api.Platform.Supported {
		spec.Run(t, "unit-analyzer/"+api.String(), testAnalyzeInputs(api.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testAnalyzeInputs(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var (
			av         *platform.AnalyzeInputsResolver
			logHandler *memory.Handler
			logger     lifecycle.Logger
		)
		it.Before(func() {
			av = &platform.AnalyzeInputsResolver{PlatformAPI: api.MustParse(platformAPI)}
			logHandler = memory.New()
			logger = &log.Logger{Handler: logHandler}
		})

		when("called without an app image", func() {
			it("errors", func() {
				_, err := av.Resolve(platform.AnalyzeInputs{}, []string{}, logger)
				h.AssertNotNil(t, err)
				expected := "failed to parse arguments: received 0 arguments, but expected 1"
				h.AssertStringContains(t, err.Error(), expected)
			})
		})

		when("latest platform api(s)", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.7"), "")
			})

			when("run image", func() {
				when("not provided", func() {
					it("falls back to stack.toml", func() {
						inputs := platform.AnalyzeInputs{
							StackPath: filepath.Join("testdata", "layers", "stack.toml"),
						}
						ret, err := av.Resolve(inputs, []string{"some-image"}, logger)
						h.AssertNil(t, err)
						h.AssertEq(t, ret.RunImageRef, "some-run-image")
					})

					when("stack.toml not present", func() {
						it("errors", func() {
							inputs := platform.AnalyzeInputs{
								StackPath: "not-exist-stack.toml",
							}
							_, err := av.Resolve(inputs, []string{"some-image"}, logger)
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
						AnalyzerOpts: platform.AnalyzerOpts{
							AdditionalTags: str.Slice{
								"some-registry.io/some-namespace/some-image:tag",
								"some-other-registry.io/some-namespace/some-image",
							},
							OutputImageRef: "some-registry.io/some-namespace/some-image",
							RunImageRef:    "some-run-image-ref", // ignore
						},
					}
					_, err := av.Resolve(inputs, []string{"some-image"}, logger)
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
					_, err := av.Resolve(platform.AnalyzeInputs{}, []string{"some-image"}, logger)
					h.AssertNil(t, err)
					expected := "Not restoring cached layer metadata, no cache flag specified."
					h.AssertLogEntry(t, logHandler, expected)
				})
			})

			when("run image", func() {
				when("not provided", func() {
					it("does not warn", func() {
						inputs := platform.AnalyzeInputs{
							StackPath: "not-exist-stack.toml",
						}
						_, err := av.Resolve(inputs, []string{"some-image"}, logger)
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
						AnalyzedPath: platform.PlaceholderAnalyzedPath,
						AnalyzerOpts: platform.AnalyzerOpts{
							LegacyGroupPath: platform.PlaceholderGroupPath,
							LayersDir:       "some-layers-dir",
						},
					}
					ret, err := av.Resolve(inputs, []string{"some-image"}, logger)
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
						AnalyzedPath: filepath.Join(".", "analyzed.toml"),
						AnalyzerOpts: platform.AnalyzerOpts{
							LegacyGroupPath: filepath.Join(".", "group.toml"),
							LayersDir:       filepath.Join("testdata", "other-layers"),
						},
					}
					ret, err := av.Resolve(inputs, []string{"some-image"}, logger)
					h.AssertNil(t, err)
					h.AssertEq(t, ret.LegacyGroupPath, filepath.Join(".", "group.toml"))
					h.AssertEq(t, ret.AnalyzedPath, filepath.Join(".", "analyzed.toml"))
				})
			})
		})
	}
}
