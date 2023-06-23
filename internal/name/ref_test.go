package name_test

import (
	"testing"

	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/internal/name"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestRef(t *testing.T) {
	spec.Run(t, "Ref", testRef)
}

func testRef(t *testing.T, when spec.G, it spec.S) {
	when(".ParseMaybe", func() {
		when("provided reference", func() {
			when("invalid", func() {
				it("returns the provided reference", func() {
					got := name.ParseMaybe("!@#$")
					h.AssertEq(t, got, "!@#$")
				})
			})

			when("has implicit registry", func() {
				it("returns the fully qualified reference", func() {
					got := name.ParseMaybe("some-library/some-repo:latest")
					h.AssertEq(t, got, "index.docker.io/some-library/some-repo:latest")
				})
			})

			when("has implicit library", func() {
				it("returns the provided reference", func() {
					got := name.ParseMaybe("some.registry/some-repo:latest")
					h.AssertEq(t, got, "some.registry/some-repo:latest")
				})

				when("registry is docker.io", func() {
					it("returns the fully qualified reference", func() {
						got := name.ParseMaybe("index.docker.io/some-repo:latest")
						h.AssertEq(t, got, "index.docker.io/library/some-repo:latest")
					})
				})
			})

			when("has implicit tag", func() {
				it("returns the fully qualified reference", func() {
					got := name.ParseMaybe("some.registry/some-library/some-repo")
					h.AssertEq(t, got, "some.registry/some-library/some-repo:latest")
				})
			})

			when("is fully qualified", func() {
				it("returns the provided reference", func() {
					got := name.ParseMaybe("some.registry/some-library/some-repo:some-tag")
					h.AssertEq(t, got, "some.registry/some-library/some-repo:some-tag")
				})
			})
		})
	})
}
