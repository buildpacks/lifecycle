// Package cache provides functionalities around the cache
package cache

import (
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/log"
)

// ImageDeleter defines the methods available to delete and compare cached images
type ImageDeleter interface {
	DeleteImage(image imgutil.Image)
	OldImageIsTheSameAsTheNewImage(oldImage imgutil.Image, newImage imgutil.Image) bool
}

// ImageDeleterImpl is a component to manage cache image deletion
type ImageDeleterImpl struct {
	logger log.Logger
}

// NewImageDeleter creates a new ImageDeleter implementation
func NewImageDeleter(logger log.Logger) ImageDeleterImpl {
	return ImageDeleterImpl{logger: logger}
}

// DeleteImage deletes an image
func (c *ImageDeleterImpl) DeleteImage(image imgutil.Image) {
	if err := image.Delete(); err != nil {
		c.logger.Warnf("Unable to delete cache image: %v", err.Error())
	}
}

// OriginAndNewImagesAreTheSame checks if the origin and the new images are the same
func (c *ImageDeleterImpl) OriginAndNewImagesAreTheSame(oldImage imgutil.Image, newImage imgutil.Image) (bool, error) {
	origIdentifier, err := oldImage.Identifier()
	if err != nil {
		return false, errors.Wrap(err, "getting identifier for original image")
	}

	newIdentifier, err := newImage.Identifier()
	if err != nil {
		return false, errors.Wrap(err, "getting identifier for new image")
	}

	if origIdentifier.String() == newIdentifier.String() {
		return true, nil
	}

	return false, nil
}
