package api_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestAPIs(t *testing.T) {
	spec.Run(t, "APIs", testAPIs, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testAPIs(t *testing.T, when spec.G, it spec.S) {
	when("NewApis", func() {
		when("deprecated contains a non-zero minor for major > 0", func() {
			it("fails", func() {
				_, err := api.NewAPIs([]string{"1.3"}, []string{"1.2"})
				h.AssertError(t, err, "invalid deprecated API '1.2'")
			})
		})

		when("supported is not a superset of deprecated", func() {
			it("fails", func() {
				_, err := api.NewAPIs([]string{"1.3"}, []string{"0.4"})
				h.AssertError(t, err, "invalid deprecated API '0.4'")
			})
		})
	})

	when("APIs", func() {
		var apis api.APIs
		it.Before(func() {
			var err error
			apis, err = api.NewAPIs([]string{"0.2", "0.3", "1.3", "2.1"}, []string{"0.2", "0.3", "1"})
			h.AssertNil(t, err)
		})

		when("IsSupported", func() {
			it("returns true if API is supported", func() {
				for _, a := range []string{"0.2", "0.3", "1", "1.0", "1.1", "1.2", "1.3", "2", "2.0", "2.1"} {
					if !apis.IsSupported(api.MustParse(a)) {
						t.Fatalf("Expected API %s to be supported", a)
					}
				}
			})

			it("returns false if API is not supported", func() {
				for _, a := range []string{"0.1", "0.4", "1.4", "2.2", "3"} {
					if apis.IsSupported(api.MustParse(a)) {
						t.Fatalf("Expected API %s NOT to be supported", a)
					}
				}
			})
		})

		when("IsDeprecated", func() {
			it("returns true if API is deprecated", func() {
				for _, a := range []string{"0.2", "0.3", "1", "1.0", "1.1", "1.2", "1.3"} {
					if !apis.IsDeprecated(api.MustParse(a)) {
						t.Fatalf("Expected API %s to be depreacted", a)
					}
				}
			})

			it("returns false if API is not deprecated", func() {
				for _, a := range []string{"2", "2.0", "2.1"} {
					if apis.IsDeprecated(api.MustParse(a)) {
						t.Fatalf("Expected API %s NOT to be deprecated", a)
					}
				}
			})
		})
	})
}
