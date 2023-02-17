package image_test

import (
	"testing"

	"github.com/docker/docker/client"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/image"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestLocalImageHandler(t *testing.T) {
	spec.Run(t, "VerifyAPIs", testLocalImageHandler, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testLocalImageHandler(t *testing.T, when spec.G, it spec.S) {
	var (
		imageHandler image.Handler
		dockerClient client.CommonAPIClient
	)

	when("Local handler", func() {
		it.Before(func() {
			dockerClient = h.DockerCli(t)
			imageHandler = image.NewHandler(dockerClient, nil, "", false)
			h.AssertNotNil(t, imageHandler)
		})

		when("#Kind", func() {
			it("return layout", func() {
				h.AssertEq(t, imageHandler.Kind(), image.LocalKind)
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
				it("creates an image", func() {
					image, err := imageHandler.InitImage("busybox")
					h.AssertNil(t, err)
					h.AssertNotNil(t, image)
					h.AssertEq(t, image.Name(), "busybox")
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
