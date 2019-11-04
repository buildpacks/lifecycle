package cache

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle"
)

const MetadataLabel = "io.buildpacks.lifecycle.cache.metadata"

type ImageCache struct {
	committed bool
	origImage imgutil.Image
	newImage  imgutil.Image
}

func NewImageCache(origImage imgutil.Image, newImage imgutil.Image) *ImageCache {
	return &ImageCache{
		origImage: origImage,
		newImage:  newImage,
	}
}

func (c *ImageCache) Name() string {
	return c.origImage.Name()
}

func (c *ImageCache) SetMetadata(metadata lifecycle.CacheMetadata) error {
	if c.committed {
		return errCacheCommitted
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return errors.Wrap(err, "serializing metadata")
	}
	return c.newImage.SetLabel(MetadataLabel, string(data))
}

func (c *ImageCache) RetrieveMetadata() (lifecycle.CacheMetadata, error) {
	var meta lifecycle.CacheMetadata
	if err := lifecycle.DecodeLabel(c.origImage, MetadataLabel, &meta); err != nil {
		return lifecycle.CacheMetadata{}, nil
	}
	return meta, nil
}

func (c *ImageCache) AddLayerFile(sha string, tarPath string) error {
	if c.committed {
		return errCacheCommitted
	}
	return c.newImage.AddLayer(tarPath)
}

func (c *ImageCache) ReuseLayer(sha string) error {
	if c.committed {
		return errCacheCommitted
	}
	return c.newImage.ReuseLayer(sha)
}

func (c *ImageCache) RetrieveLayer(sha string) (io.ReadCloser, error) {
	return c.origImage.GetLayer(sha)
}

func (c *ImageCache) Commit() error {
	if c.committed {
		return errCacheCommitted
	}

	if err := c.newImage.Save(); err != nil {
		return errors.Wrapf(err, "saving image '%s'", c.newImage.Name())
	}
	c.committed = true

	if c.origImage.Found() {
		// Deleting the original image is for cleanup only and should not fail the commit.
		if err := c.origImage.Delete(); err != nil {
			fmt.Printf("Unable to delete previous cache image: %v", err)
		}
	}
	c.origImage = c.newImage
	return nil
}
