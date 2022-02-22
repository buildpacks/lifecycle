package platform_test

import (
	h "github.com/buildpacks/lifecycle/testhelpers"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
)

func TestAnalyzerBuilder(t *testing.T) {
	for _, api := range api.Platform.Supported {
		spec.Run(t, "unit-analyzer/"+api.String(), testAnalyzerBuilder(api.String()), spec.Parallel(), spec.Report(report.Terminal{}))
	}
}

func testAnalyzerBuilder(platformAPI string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		when("platform api >= 0.8", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.8"), "")
			})

			it("restores sbom layers from the previous image", func() {

			})

			when("previous image", func() {
				it("provides it to the analyzer", func() {

				})

				when("daemon case", func() {
					when("provided a launch cache dir", func() {
						it("previous image is a caching image", func() {

						})
					})
				})
			})
		})

		when("platform api >= 0.7", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).LessThan("0.7"), "")
			})

			when("provided a group", func() {
				it("ignores it", func() {

				})
			})

			when("provided a cache image", func() {
				it("validates registry access", func() {

				})
			})

			when("provided a cache directory", func() {
				it("ignores it", func() {

				})
			})

			it("does not restore layer metadata", func() {

			})

			when("previous image", func() {
				it("provides it to the analyzer", func() {

				})
			})

			when("run image", func() {
				it("provides it to the analyzer", func() {

				})
			})

			it("does not restore sbom layers from the previous image", func() {

			})
		})

		when("platform api < 0.7", func() {
			it.Before(func() {
				h.SkipIf(t, api.MustParse(platformAPI).AtLeast("0.7"), "")
			})

			when("provided a group", func() {
				it("reads group.toml", func() {

				})

				it("validates buildpack apis", func() {

				})
			})

			when("provided a cache image", func() {
				it("provides it to the analyzer", func() {

				})
			})

			when("provided a cache directory", func() {
				it("provides it to the analyzer", func() {

				})
			})

			it("restores layer metadata", func() {

			})

			when("previous image", func() {
				it("provides it to the analyzer", func() {

				})
			})

			when("provided a run image", func() {
				it("ignores it", func() {

				})
			})

			it("does not restore sbom layers from the previous image", func() {

			})
		})
	}
}
