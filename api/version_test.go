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

	when("IsSupersetOf", func() {
		when("0.x", func() {
			it("matching Minor value", func() {
				v := api.MustParse("0.2")
				target := api.MustParse("0.2")

				h.AssertEq(t, v.IsSupersetOf(target), true)
			})

			it("Minor > target Minor", func() {
				v := api.MustParse("0.2")
				target := api.MustParse("0.1")

				h.AssertEq(t, v.IsSupersetOf(target), false)
			})

			it("Minor < target Minor", func() {
				v := api.MustParse("0.1")
				target := api.MustParse("0.2")

				h.AssertEq(t, v.IsSupersetOf(target), false)
			})
		})

		when("1.x", func() {
			it("matching Major and Minor", func() {
				v := api.MustParse("1.2")
				target := api.MustParse("1.2")

				h.AssertEq(t, v.IsSupersetOf(target), true)
			})

			it("matching Major but Minor > target Minor", func() {
				v := api.MustParse("1.2")
				target := api.MustParse("1.1")

				h.AssertEq(t, v.IsSupersetOf(target), true)
			})

			it("matching Major but Minor < target Minor", func() {
				v := api.MustParse("1.1")
				target := api.MustParse("1.2")

				h.AssertEq(t, v.IsSupersetOf(target), false)
			})

			it("Major < target Major", func() {
				v := api.MustParse("1.0")
				target := api.MustParse("2.0")

				h.AssertEq(t, v.IsSupersetOf(target), false)
			})

			it("Major > target Major", func() {
				v := api.MustParse("2.0")
				target := api.MustParse("1.0")

				h.AssertEq(t, v.IsSupersetOf(target), false)
			})
		})
	})

	when("#LessThan", func() {
		var subject = api.MustParse("0.3")
		var toTest = map[string]bool{
			"0.2": false,
			"0.3": false,
			"0.4": true,
		}
		it("returns the expected value", func() {
			for comparison, expected := range toTest {
				h.AssertEq(t, subject.LessThan(comparison), expected)
			}
		})
	})

	when("#AtLeast", func() {
		var subject = api.MustParse("0.3")
		var toTest = map[string]bool{
			"0.2": true,
			"0.3": true,
			"0.4": false,
		}
		it("returns the expected value", func() {
			for comparison, expected := range toTest {
				h.AssertEq(t, subject.AtLeast(comparison), expected)
			}
		})
	})
}
