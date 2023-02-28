package image_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/lifecycle/image"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

const (
	defaultDockerRegistry = name.DefaultRegistry
	defaultDockerRepo     = "library"
)

func TestLayoutImageHandler(t *testing.T) {
	spec.Run(t, "VerifyAPIs", testLayoutImageHandler, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testLayoutImageHandler(t *testing.T, when spec.G, it spec.S) {
	var (
		imageHandler image.Handler
		layoutDir    string
		imageRef     string
		imageDigest  string
		imageTag     string
	)

	when("layout handler", func() {
		it.Before(func() {
			layoutDir = "layout-repo"
			imageHandler = image.NewHandler(nil, nil, layoutDir, true)
			h.AssertNotNil(t, imageHandler)
		})

		when("#Kind", func() {
			it("returns layout", func() {
				h.AssertEq(t, imageHandler.Kind(), image.LayoutKind)
			})
		})

		when("#InitImage", func() {
			when("no image reference is provided", func() {
				it("nil image is return", func() {
					image, err := imageHandler.InitImage("")
					h.AssertNil(t, err)
					h.AssertNil(t, image)
				})
			})

			when("image reference is provided", func() {
				when("no tag or digest are provided", func() {
					it.Before(func() {
						imageRef = "my-full-stack-run"
						imageTag = "latest"
					})

					it("creates image path with defaults and latest tag", func() {
						image, err := imageHandler.InitImage(imageRef)
						h.AssertNil(t, err)
						h.AssertEq(t, image.Name(), filepath.Join(layoutDir, defaultDockerRegistry, defaultDockerRepo, imageRef, imageTag))
					})
				})

				when("tag is provided", func() {
					it.Before(func() {
						imageRef = "my-full-stack-run"
						imageTag = "bionic"
					})

					it("creates image path with defaults and tag provided", func() {
						image, err := imageHandler.InitImage(tag(imageRef, imageTag))
						h.AssertNil(t, err)
						h.AssertEq(t, image.Name(), filepath.Join(layoutDir, defaultDockerRegistry, defaultDockerRepo, imageRef, imageTag))
					})
				})

				when("digest is provided", func() {
					it.Before(func() {
						imageRef = "my-full-stack-run"
						imageDigest = "f75f3d1a317fc82c793d567de94fc8df2bece37acd5f2bd364a0d91a0d1f3dab"
					})

					it("creates image path with defaults and digest provided", func() {
						image, err := imageHandler.InitImage(sha256(imageRef, imageDigest))
						h.AssertNil(t, err)
						h.AssertEq(t, image.Name(), filepath.Join(layoutDir, defaultDockerRegistry, defaultDockerRepo, imageRef, "sha256", imageDigest))
					})
				})
			})

			when("image reference is not well formed", func() {
				it("err is return", func() {
					_, err := imageHandler.InitImage("my-bad-image-reference::latest")
					h.AssertNotNil(t, err)
				})
			})
		})
	})
}

func tag(image, tag string) string {
	return fmt.Sprintf("%s:%s", image, tag)
}

func sha256(image, digest string) string {
	return fmt.Sprintf("%s@sha256:%s", image, digest)
}
