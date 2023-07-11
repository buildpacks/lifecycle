package cache

import (
	"testing"

	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestImageComparer(t *testing.T) {
	spec.Run(t, "ImageComparer", testImageComparer, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testImageComparer(t *testing.T, when spec.G, it spec.S) {
	var (
		imageComparer ImageComparer
	)

	it.Before(func() {
		imageComparer = NewImageComparer()
	})

	when("Comparing two images: orig and new", func() {
		it("checks if the images have the same identifier", func() {
			fakeOldImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeOldImage"})
			fakeNewImage := fakes.NewImage("fake-new-image", "", local.IDIdentifier{ImageID: "fakeNewImage"})

			result, _ := imageComparer.ImagesEq(fakeOldImage, fakeNewImage)

			h.AssertEq(t, result, false)
		})

		it("returns an error if it's impossible to get the original image identifier", func() {
			fakeOriginalImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeOriginalImage"})
			fakeNewImage := fakes.NewImage("fake-new-image", "", local.IDIdentifier{ImageID: "fakeNewImage"})
			fakeErrorImage := newFakeImageErrIdentifier(fakeOriginalImage, "original")

			_, err := imageComparer.ImagesEq(fakeErrorImage, fakeNewImage)

			h.AssertError(t, err, "getting identifier for original image")
		})

		it("returns an error if it's impossible to get the new image identifier", func() {
			fakeOriginalImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeOriginalImage"})
			fakeNewImage := fakes.NewImage("fake-new-image", "", local.IDIdentifier{ImageID: "fakeNewImage"})
			fakeErrorImage := newFakeImageErrIdentifier(fakeNewImage, "new")

			_, err := imageComparer.ImagesEq(fakeOriginalImage, fakeErrorImage)

			h.AssertError(t, err, "getting identifier for new image")
		})
	})
}
