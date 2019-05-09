package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/buildpack/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/cache"
)

type cachingImage struct {
	image imgutil.Image
	cache *cache.VolumeCache
}

func NewCachingImage(image imgutil.Image, cache *cache.VolumeCache) imgutil.Image {
	return &cachingImage{
		image: image,
		cache: cache,
	}
}

func (c *cachingImage) AddLayer(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrap(err, "opening layer file")
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return errors.Wrap(err, "hashing layer")
	}
	sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))

	if err := c.cache.AddLayerFile("sha256:"+sha, path); err != nil {
		return err
	}

	return c.image.AddLayer(path)
}

func (c *cachingImage) ReuseLayer(sha string) error {
	if found, err := c.cache.HasLayer(sha); err != nil {
		return err
	} else if found {
		if err := c.cache.ReuseLayer(sha); err != nil {
			return err
		}
		path, err := c.cache.RetrieveLayerFile(sha)
		if err != nil {
			return err
		}
		return c.image.AddLayer(path)
	} else {
		if err := c.image.ReuseLayer(sha); err != nil {
			return err
		}
		rc, err := c.image.GetLayer(sha)
		if err != nil {
			return err
		}
		return c.cache.AddLayer(rc)
	}
}

func (c *cachingImage) GetLayer(sha string) (io.ReadCloser, error) {
	if found, err := c.cache.HasLayer(sha); err != nil {
		return nil, fmt.Errorf("cache no layer with sha '%s'", sha)
	} else if found {
		return c.cache.RetrieveLayer(sha)
	} else {
		return c.image.GetLayer(sha)
	}
}

func (c *cachingImage) Save() (string, error) {
	sha, err := c.image.Save()
	if err != nil {
		return "", err
	}
	if err := c.cache.Commit(); err != nil {
		return "", err
	}
	return sha, nil
}

// delegates to image

func (c *cachingImage) Name() string {
	return c.image.Name()
}

func (c *cachingImage) Rename(name string) {
	c.image.Rename(name)
}

func (c *cachingImage) Digest() (string, error) {
	return c.image.Digest()
}

func (c *cachingImage) Label(label string) (string, error) {
	return c.image.Label(label)
}

func (c *cachingImage) SetLabel(key, value string) error {
	return c.image.SetLabel(key, value)
}

func (c *cachingImage) Env(key string) (string, error) {
	return c.image.Env(key)
}

func (c *cachingImage) SetEnv(key, value string) error {
	return c.image.SetEnv(key, value)
}

func (c *cachingImage) SetEntrypoint(entrypoint ...string) error {
	return c.image.SetEntrypoint(entrypoint...)
}

func (c *cachingImage) SetWorkingDir(wd string) error {
	return c.image.SetWorkingDir(wd)
}

func (c *cachingImage) SetCmd(cmd ...string) error {
	return c.image.SetCmd(cmd...)
}

func (c *cachingImage) Rebase(topLayer string, newBase imgutil.Image) error {
	return c.image.Rebase(topLayer, newBase)
}

func (c *cachingImage) TopLayer() (string, error) {
	return c.image.TopLayer()
}

func (c *cachingImage) Found() (bool, error) {
	return c.image.Found()
}

func (c *cachingImage) Delete() error {
	return c.image.Delete()
}

func (c *cachingImage) CreatedAt() (time.Time, error) {
	return c.image.CreatedAt()
}
