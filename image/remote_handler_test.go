package image_test

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/image"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestRemoteImageHandler(t *testing.T) {
	spec.Run(t, "VerifyAPIs", testRemoteImageHandler, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testRemoteImageHandler(t *testing.T, when spec.G, it spec.S) {
	var (
		imageHandler image.Handler
		auth         authn.Keychain
	)

	when("Remote handler", func() {
		it.Before(func() {
			auth = authn.DefaultKeychain
			imageHandler = image.NewHandler(nil, auth, "", false, "host.docker.internal")
			h.AssertNotNil(t, imageHandler)
		})

		when("#Kind", func() {
			it("returns remote", func() {
				h.AssertEq(t, imageHandler.Kind(), image.RemoteKind)
			})
		})

		when("#InitImage", func() {
			when("no image reference is provided", func() {
				it("nil image is return", func() {
					newImage, err := imageHandler.InitImage("")
					h.AssertNil(t, err)
					h.AssertNil(t, newImage)
				})
			})

			when("image reference is provided", func() {
				it("creates an image", func() {
					newImage, err := imageHandler.InitImage("busybox")
					h.AssertNil(t, err)
					h.AssertNotNil(t, newImage)
					h.AssertEq(t, newImage.Name(), "busybox")
				})
				it("creates an image using an insecure registry", func() {
					_, err := imageHandler.InitImage("host.docker.internal/bar")
					h.AssertNotNil(t, err)
					h.AssertError(t, err, "http://")
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
