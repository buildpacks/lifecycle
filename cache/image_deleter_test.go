package cache

import (
	"testing"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/log"
	h "github.com/buildpacks/lifecycle/testhelpers"
	cacheMock "github.com/buildpacks/lifecycle/testmock/cache"
)

func TestImageDeleter(t *testing.T) {
	spec.Run(t, "ImageDeleter", testImageDeleter, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testImageDeleter(t *testing.T, when spec.G, it spec.S) {
	var (
		testLogger        log.Logger
		imageDeleter      ImageDeleter
		fakeImageComparer *cacheMock.MockImageComparer
	)

	it.Before(func() {
		testLogger = cmd.DefaultLogger
		mockController := gomock.NewController(t)
		fakeImageComparer = cacheMock.NewMockImageComparer(mockController)
		imageDeleter = NewImageDeleter(fakeImageComparer, testLogger, true)
	})

	when("delete functionality has ben activated", func() {
		it("should delete the image when provided and orig and new images are differents", func() {
			fakeOrigImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeImage"})
			fakeNewImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeNewImage"})
			fakeImageComparer.EXPECT().ImagesEq(fakeOrigImage, fakeNewImage).AnyTimes().Return(false, nil)

			imageDeleter.DeleteOrigImageIfDifferentFromNewImage(fakeOrigImage, fakeNewImage)

			h.AssertEq(t, fakeOrigImage.Found(), false)
		})

		it("should raise a warning if delete doesn't work properly", func() {
			fakeNewImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeNewImage"})
			fakeOriginalErrorImage := newFakeImageErrIdentifier(fakeNewImage, "original")
			mockLogger := &MockLogger{Logger: cmd.DefaultLogger}
			fakeImageComparer.EXPECT().ImagesEq(fakeOriginalErrorImage, fakeNewImage).AnyTimes().Return(false, nil)
			imageDeleter := NewImageDeleter(fakeImageComparer, mockLogger, true)

			imageDeleter.DeleteOrigImageIfDifferentFromNewImage(fakeOriginalErrorImage, fakeNewImage)

			h.AssertEq(t, mockLogger.Calls, 1)
		})

		when("comparing two images: orig and new and they are the same", func() {
			it("should not perform deleting operations", func() {
				fakeOrigImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeImage"})
				fakeNewImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeImage"})
				fakeImageComparer.EXPECT().ImagesEq(fakeOrigImage, fakeNewImage).AnyTimes().Return(true, nil)
				imageDeleter = NewImageDeleter(fakeImageComparer, testLogger, true)

				imageDeleter.DeleteOrigImageIfDifferentFromNewImage(fakeOrigImage, fakeNewImage)

				h.AssertEq(t, fakeOrigImage.Found(), true)
			})
		})
	})

	when("delete functionality has been deactivated", func() {
		it("should not perform deleting operations", func() {
			fakeOrigImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeImage"})
			fakeNewImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeImage"})
			imageDeleter = NewImageDeleter(fakeImageComparer, testLogger, false)

			imageDeleter.DeleteOrigImageIfDifferentFromNewImage(fakeOrigImage, fakeNewImage)

			h.AssertEq(t, fakeOrigImage.Found(), true)
		})
	})
}

type fakeErrorImage struct {
	imgutil.Image
	typeOfImage string
}

func newFakeImageErrIdentifier(fakeImage *fakes.Image, typeOfImage string) *fakeErrorImage {
	return &fakeErrorImage{Image: fakeImage, typeOfImage: typeOfImage}
}

func (f *fakeErrorImage) Identifier() (imgutil.Identifier, error) {
	return nil, errors.New("error deleting " + f.typeOfImage + " image")
}

func (f *fakeErrorImage) Delete() error {
	return errors.New("fakeError")
}

type MockLogger struct {
	log.Logger
	Calls int
}

func (l *MockLogger) Warnf(string, ...interface{}) { l.Calls++ }
