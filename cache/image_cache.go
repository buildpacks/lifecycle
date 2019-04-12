package cache

import (
	"encoding/json"
	"io"
	"log"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/image"
	"github.com/pkg/errors"
)

//go:generate mockgen -package testmock -destination testmock/image_factory.go github.com/buildpack/lifecycle/cache ImageFactory

type ImageFactory interface {
	NewEmptyLocal(string) image.Image
}

type ImageCache struct {
	logger    *log.Logger
	factory   ImageFactory
	origImage image.Image
	newImage  *image.LoggingImage
}

func NewImageCache(logger *log.Logger, factory ImageFactory, origImage image.Image) *ImageCache {
	newImage := factory.NewEmptyLocal(origImage.Name())
	return &ImageCache{
		logger:    logger,
		factory:   factory,
		origImage: origImage,
		newImage:  image.NewLoggingImage(logger, newImage),
	}
}

func (c *ImageCache) Name() string {
	return c.origImage.Name()
}

func (c *ImageCache) SetMetadata(metadata lifecycle.CacheMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return errors.Wrap(err, "marshall metadata")
	}
	return c.newImage.SetLabel(lifecycle.CacheMetadataLabel, string(data))
}

func (c *ImageCache) RetrieveMetadata() (lifecycle.CacheMetadata, bool, error) {
	contents, err := lifecycle.GetMetadata(c.origImage, lifecycle.CacheMetadataLabel, c.logger)
	if err != nil {
		return lifecycle.CacheMetadata{}, false, err
	}

	metadata := lifecycle.CacheMetadata{}
	if err := json.Unmarshal([]byte(contents), &metadata); err != nil {
		c.logger.Printf("WARNING: image '%s' has incompatible '%s' label\n", c.origImage.Name(), lifecycle.CacheMetadataLabel)
		return lifecycle.CacheMetadata{}, false, nil
	}

	return metadata, true, nil
}

func (c *ImageCache) AddLayer(identifier string, sha string, tarPath string) error {
	return c.newImage.AddLayer(identifier, sha, tarPath)
}

func (c *ImageCache) ReuseLayer(identifier string, sha string) error {
	return c.newImage.ReuseLayer(identifier, sha)
}

func (c *ImageCache) RetrieveLayer(sha string) (io.ReadCloser, error) {
	return c.origImage.GetLayer(sha)
}

func (c *ImageCache) Commit() error {
	sha, err := c.newImage.Save()
	if err != nil {
		return err
	}
	c.logger.Printf("cache '%s@%s'\n", c.newImage.Name(), sha)

	if err := c.origImage.Delete(); err != nil {
		return err
	}

	c.origImage = c.newImage.Image
	c.newImage = image.NewLoggingImage(c.logger, c.factory.NewEmptyLocal(c.origImage.Name()))

	return nil
}
