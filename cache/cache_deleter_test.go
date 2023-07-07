package cache

import (
	"testing"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/imgutil/local"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/log"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestCacheDeleter(t *testing.T) {
	spec.Run(t, "Exporter", testCacheDeleter, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testCacheDeleter(t *testing.T, when spec.G, it spec.S) {
	var (
		testLogger   log.Logger
		cacheDeleter ImageDeleterImpl
	)

	it.Before(func() {
		testLogger = cmd.DefaultLogger
		cacheDeleter = NewImageDeleter(testLogger)
	})

	it("should delete the image when provided", func() {
		fakeImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeImage"})

		cacheDeleter.DeleteImage(fakeImage)

		h.AssertEq(t, fakeImage.Found(), false)
	})

	it("should raise a warning if delete doesn't work properly", func() {
		fakeImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeImage"})
		fakeErrorImage := newFakeImageErrIdentifier(fakeImage, "original")
		mockLogger := &MockLogger{Logger: cmd.DefaultLogger}
		cacheDeleter := NewImageDeleter(mockLogger)

		cacheDeleter.DeleteImage(fakeErrorImage)

		h.AssertEq(t, mockLogger.Calls, 1)
	})

	when("Comparing two images: orig and new", func() {
		it("doesn't do anything", func() {
			fakeOldImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeOldImage"})
			fakeNewImage := fakes.NewImage("fake-new-image", "", local.IDIdentifier{ImageID: "fakeNewImage"})
			cacheDeleter := NewImageDeleter(testLogger)

			result, _ := cacheDeleter.OriginAndNewImagesAreTheSame(fakeOldImage, fakeNewImage)

			h.AssertEq(t, result, false)
		})

		it("Should returns an error if it's impossible to get the original image identifier", func() {
			fakeOriginalImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeOriginalImage"})
			fakeNewImage := fakes.NewImage("fake-new-image", "", local.IDIdentifier{ImageID: "fakeNewImage"})
			fakeErrorImage := newFakeImageErrIdentifier(fakeOriginalImage, "original")
			cacheDeleter := NewImageDeleter(testLogger)

			_, err := cacheDeleter.OriginAndNewImagesAreTheSame(fakeErrorImage, fakeNewImage)

			h.AssertError(t, err, "getting identifier for original image")
		})

		it("Should returns an error if it's impossible to get the new image identifier", func() {
			fakeOriginalImage := fakes.NewImage("fake-image", "", local.IDIdentifier{ImageID: "fakeOriginalImage"})
			fakeNewImage := fakes.NewImage("fake-new-image", "", local.IDIdentifier{ImageID: "fakeNewImage"})
			fakeErrorImage := newFakeImageErrIdentifier(fakeNewImage, "new")
			cacheDeleter := NewImageDeleter(testLogger)

			_, err := cacheDeleter.OriginAndNewImagesAreTheSame(fakeOriginalImage, fakeErrorImage)

			h.AssertError(t, err, "getting identifier for new image")
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
