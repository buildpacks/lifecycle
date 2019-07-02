package image_test

import (
	"testing"

	"github.com/sclevine/spec"

	"github.com/buildpack/lifecycle/image"
	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestImage(t *testing.T) {
	spec.Run(t, "Test Image", testImage)
}

func testImage(t *testing.T, when spec.G, it spec.S) {
	when("ByRegistry", func() {
		var images []string
		it.Before(func() {
			images = []string{
				"first.com/org/repo",
				"myorg/myrepo",
				"zonal.gcr.io/org/repo",
				"gcr.io/org/repo",
			}
		})

		when("repoName is dockerhub", func() {
			it("returns the dockerhub image", func() {
				name, err := image.ByRegistry("index.docker.io", images)
				h.AssertNil(t, err)
				h.AssertEq(t, name, "myorg/myrepo")
			})
		})

		when("registry is gcr.io", func() {
			it("returns the gcr.io image", func() {
				name, err := image.ByRegistry("gcr.io", images)
				h.AssertNil(t, err)
				h.AssertEq(t, name, "gcr.io/org/repo")
			})

			when("registry is zonal.gcr.io", func() {
				it("returns the gcr image", func() {
					name, err := image.ByRegistry("zonal.gcr.io", images)
					h.AssertNil(t, err)
					h.AssertEq(t, name, "zonal.gcr.io/org/repo")
				})
			})

			when("registry is missingzone.gcr.io", func() {
				it("returns first run image", func() {
					name, err := image.ByRegistry("missingzone.gcr.io", images)
					h.AssertNil(t, err)
					h.AssertEq(t, name, "first.com/org/repo")
				})
			})
		})

		when("one of the images is non-parsable", func() {
			it.Before(func() {
				images = []string{"as@ohd@as@op", "gcr.io/myorg/myrepo"}
			})

			it("skips over it", func() {
				name, err := image.ByRegistry("gcr.io", images)
				h.AssertNil(t, err)
				h.AssertEq(t, name, "gcr.io/myorg/myrepo")
			})
		})
	})

	when("#EnsureSingleRegistry", func() {
		when("multiple registries are provided", func() {
			it("errors as unsupported", func() {
				_, err := image.EnsureSingleRegistry("some/repo", "gcr.io/other-repo:latest", "example.com/final-repo")
				h.AssertError(t, err, "exporting to multiple registries is unsupported")
			})
		})

		when("a single registry is provided", func() {
			it("does not return an error and", func() {
				registry, err := image.EnsureSingleRegistry("gcr.io/some/repo", "gcr.io/other-repo:latest", "gcr.io/final-repo")
				h.AssertNil(t, err)

				h.AssertEq(t, registry, "gcr.io")
			})
		})
	})
}
