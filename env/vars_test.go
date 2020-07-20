package env_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/env"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestVars(t *testing.T) {
	spec.Run(t, "Vars", testVars, spec.Report(report.Terminal{}))
}

func testVars(t *testing.T, when spec.G, it spec.S) {
	when("#NewVars", func() {
		when("case sensitive", func() {
			it("should load values as is", func() {
				m := env.NewVars(
					map[string]string{
						"foo": "bar",
					},
					false,
				)

				h.AssertEq(t, m.Get("foo"), "bar")
				h.AssertEq(t, m.Get("Foo"), "")
			})
		})

		when("case insensitive", func() {
			it("should load values normalized", func() {
				m := env.NewVars(
					map[string]string{
						"foo": "bar",
					},
					true,
				)

				h.AssertEq(t, m.Get("foo"), "bar")
				h.AssertEq(t, m.Get("Foo"), "bar")
			})
		})
	})

	when("#Set", func() {
		when("case sensitive", func() {
			it("should set value as is", func() {
				m := env.NewVars(nil, false)
				m.Set("foo", "bar")

				h.AssertEq(t, m.Get("foo"), "bar")
				h.AssertEq(t, m.Get("Foo"), "")
			})
		})

		when("case insensitive", func() {
			it("should set value normalized", func() {
				m := env.NewVars(nil, true)
				m.Set("foo", "bar")

				h.AssertEq(t, m.Get("foo"), "bar")
				h.AssertEq(t, m.Get("Foo"), "bar")
			})
		})
	})

	when("#Values", func() {
		when("case sensitive", func() {
			it("should load values as is", func() {
				m := env.NewVars(
					map[string]string{
						"foo": "bar",
						"baz": "taz",
					},
					false,
				)

				h.AssertContains(t, m.List(), "foo=bar", "baz=taz")
			})
		})

		when("case insensitive", func() {
			it("should load values normalized", func() {
				m := env.NewVars(
					map[string]string{
						"foo": "bar",
						"baz": "taz",
					},
					true,
				)

				h.AssertContains(t, m.List(), "FOO=bar", "BAZ=taz")
			})
		})
	})
}
