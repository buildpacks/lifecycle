package api_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestAPIVersion(t *testing.T) {
	spec.Run(t, "APIVersion", testAPIVersion, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testAPIVersion(t *testing.T, when spec.G, it spec.S) {
	when("#Equal", func() {
		it("is equal to comparison", func() {
			subject := api.MustParse("0.2")
			comparison := api.MustParse("0.2")

			h.AssertEq(t, subject.Equal(comparison), true)
		})

		it("is not equal to comparison", func() {
			subject := api.MustParse("0.2")
			comparison := api.MustParse("0.3")

			h.AssertEq(t, subject.Equal(comparison), false)
		})
	})

	when("IsAPICompatible", func() {
		when("pre-stable", func() {
			it("matching minor value", func() {
				lifecycle := api.MustParse("0.2")
				platform := api.MustParse("0.2")

				h.AssertEq(t, api.IsAPICompatible(lifecycle, platform), true)
			})

			it("lifecycle minor > platform minor", func() {
				lifecycle := api.MustParse("0.2")
				platform := api.MustParse("0.1")

				h.AssertEq(t, api.IsAPICompatible(lifecycle, platform), false)
			})

			it("lifecycle minor < platform minor", func() {
				lifecycle := api.MustParse("0.1")
				platform := api.MustParse("0.2")

				h.AssertEq(t, api.IsAPICompatible(lifecycle, platform), false)
			})
		})

		when("stable", func() {
			it("matching major and minor", func() {
				lifecycle := api.MustParse("1.2")
				comparison := api.MustParse("1.2")

				h.AssertEq(t, api.IsAPICompatible(lifecycle, comparison), true)
			})

			it("matching major but minor > platform minor", func() {
				lifecycle := api.MustParse("1.2")
				platform := api.MustParse("1.1")

				h.AssertEq(t, api.IsAPICompatible(lifecycle, platform), true)
			})

			it("matching major but minor < platform minor", func() {
				lifecycle := api.MustParse("1.1")
				platform := api.MustParse("1.2")

				h.AssertEq(t, api.IsAPICompatible(lifecycle, platform), false)
			})

			it("major < platform major", func() {
				lifecycle := api.MustParse("1.0")
				platform := api.MustParse("2.0")

				h.AssertEq(t, api.IsAPICompatible(lifecycle, platform), false)
			})

			it("major > platform major", func() {
				lifecycle := api.MustParse("2.0")
				platform := api.MustParse("1.0")

				h.AssertEq(t, api.IsAPICompatible(lifecycle, platform), false)
			})
		})
	})
}
