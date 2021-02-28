package cache

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/platform"
)

const MetadataLabel = "io.buildpacks.lifecycle.cache.metadata"

type ImageCache struct {
	committed         bool
	origImage         imgutil.Image
	newImage          imgutil.Image
	generatedLayerDir string
}

func NewImageCache(origImage imgutil.Image, newImage imgutil.Image) (*ImageCache, error) {
	generatedLayerDir, err := ioutil.TempDir("", "image-cache-generated-layers")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}

	return &ImageCache{
		origImage:         origImage,
		newImage:          newImage,
		generatedLayerDir: generatedLayerDir,
	}, nil
}

func NewImageCacheFromName(name string, keychain authn.Keychain) (*ImageCache, error) {
	origImage, err := remote.NewImage(
		name,
		keychain,
		remote.FromBaseImage(name),
		remote.WithPlatform(imgutil.Platform{OS: runtime.GOOS}),
	)
	if err != nil {
		return nil, fmt.Errorf("accessing cache image %q: %v", name, err)
	}
	emptyImage, err := remote.NewImage(
		name,
		keychain,
		remote.WithPreviousImage(name),
		remote.WithPlatform(imgutil.Platform{OS: runtime.GOOS}),
	)
	if err != nil {
		return nil, fmt.Errorf("creating new cache image %q: %v", name, err)
	}

	return NewImageCache(origImage, emptyImage)
}

func (c *ImageCache) Exists() bool {
	return c.origImage.Found()
}

func (c *ImageCache) Name() string {
	return c.origImage.Name()
}

func (c *ImageCache) SetMetadata(metadata platform.CacheMetadata) error {
	if c.committed {
		return errCacheCommitted
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return errors.Wrap(err, "serializing metadata")
	}
	return c.newImage.SetLabel(MetadataLabel, string(data))
}

func (c *ImageCache) RetrieveMetadata() (platform.CacheMetadata, error) {
	var meta platform.CacheMetadata
	if err := lifecycle.DecodeLabel(c.origImage, MetadataLabel, &meta); err != nil {
		return platform.CacheMetadata{}, nil
	}
	return meta, nil
}

func (c *ImageCache) AddLayerFile(tarPath string, diffID string) error {
	if c.committed {
		return errCacheCommitted
	}
	return c.newImage.AddLayerWithDiffID(tarPath, diffID)
}

func (c *ImageCache) ReuseLayer(diffID string) error {
	if c.committed {
		return errCacheCommitted
	}
	return c.newImage.ReuseLayer(diffID)
}

func (c *ImageCache) RetrieveLayer(diffID string) (io.ReadCloser, error) {
	return c.origImage.GetLayer(diffID)
}

func (c *ImageCache) Commit() error {
	if c.committed {
		return errCacheCommitted
	}

	// Check if the cache image exists prior to saving the new cache at that same location
	origImgExists := c.origImage.Found()

	if err := c.newImage.Save(); err != nil {
		return errors.Wrapf(err, "saving image '%s'", c.newImage.Name())
	}
	c.committed = true

	if origImgExists {
		// Deleting the original image is for cleanup only and should not fail the commit.
		if err := c.DeleteOrigImage(); err != nil {
			fmt.Printf("Unable to delete previous cache image: %v", err)
		}
	}
	c.origImage = c.newImage

	// Deleting generated layers is for cleanup only and should not fail the commit.
	if _, err := os.Stat(c.generatedLayerDir); !os.IsNotExist(err) {
		if err != os.RemoveAll(c.generatedLayerDir) {
			fmt.Printf("Unable to delete generated layer: %v", err)
		}
	}

	return nil
}

func (c *ImageCache) DeleteOrigImage() error {
	origIdentifier, err := c.origImage.Identifier()
	if err != nil {
		return errors.Wrap(err, "getting identifier for original image")
	}
	newIdentifier, err := c.newImage.Identifier()
	if err != nil {
		return errors.Wrap(err, "getting identifier for new image")
	}
	if origIdentifier.String() == newIdentifier.String() {
		return nil
	}
	return c.origImage.Delete()
}
