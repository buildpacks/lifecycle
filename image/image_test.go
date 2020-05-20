package image_test

import (
	"testing"

	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/image"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestImage(t *testing.T) {
	spec.Run(t, "Test Image", testImage)
}

func testImage(t *testing.T, when spec.G, it spec.S) {
	when("#ValidateDestinationTags", func() {
		when("multiple registries are provided", func() {
			when("daemon", func() {
				it("does not return an error", func() {
					err := image.ValidateDestinationTags(true, "some/repo", "gcr.io/other-repo:latest", "example.com/final-repo")
					h.AssertNil(t, err)
				})
			})
			when("registry", func() {
				it("errors as unsupported", func() {
					err := image.ValidateDestinationTags(false, "some/repo", "gcr.io/other-repo:latest", "example.com/final-repo")
					h.AssertError(t, err, "writing to multiple registries is unsupported")
				})
			})
		})

		when("a single registry is provided", func() {
			it("does not return an error", func() {
				err := image.ValidateDestinationTags(false, "gcr.io/some/repo", "gcr.io/other-repo:latest", "gcr.io/final-repo")
				h.AssertNil(t, err)
			})
		})

		when("the tag reference is invalid", func() {
			it("errors", func() {
				err := image.ValidateDestinationTags(false, "some/Repo")
				h.AssertError(t, err, "could not parse reference: some/Repo")
			})
		})
	})
}
