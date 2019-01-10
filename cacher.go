package lifecycle

import (
	"encoding/json"
	"log"
	"sort"

	"github.com/buildpack/lifecycle/image"
	"github.com/pkg/errors"
)

type Cacher interface {
	RequiresTar() bool
	Export(*AppImageMetadata) error
}

type LocalImageCacher struct {
	BaseImage image.Image
	RepoName  string
	Out       *log.Logger
}

type NoopCacher struct {
}

func NewLocalImageCacher(cacheImageRef, repoName string, factory image.Factory, out *log.Logger) (Cacher, error) {
	baseImage, err := factory.NewLocal(cacheImageRef, false)
	if err != nil {
		return nil, err
	}

	return &LocalImageCacher{
		BaseImage: baseImage,
		RepoName:  repoName,
		Out:       out,
	}, nil
}

func (c *NoopCacher) Export(_ *AppImageMetadata) error {
	return nil
}

func (c *NoopCacher) RequiresTar() bool {
	return false
}

func (c *LocalImageCacher) Export(metadata *AppImageMetadata) error {
	var err error

	cacheImage := c.BaseImage
	cacheImage.Rename(c.RepoName)

	for _, bp := range metadata.Buildpacks {
		layerKeys := make([]string, 0, len(bp.Layers))
		for n, _ := range bp.Layers {
			layerKeys = append(layerKeys, n)
		}
		sort.Strings(layerKeys)

		for _, layerName := range layerKeys {
			layer := bp.Layers[layerName]

			if layer.Cache {
				c.Out.Printf("adding cache layer '%s/%s' with diffID '%s'\n", bp.ID, layerName, layer.SHA)
				if err := cacheImage.AddLayer(layer.Tar); err != nil {
					return err
				}
			}
		}
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return errors.Wrap(err, "marshall metadata")
	}
	c.Out.Printf("setting cache metadata label '%s'\n", MetadataLabel)
	if err := cacheImage.SetLabel(MetadataLabel, string(data)); err != nil {
		return errors.Wrap(err, "set cache image metadata label")
	}

	c.Out.Println("writing cache image")
	sha, err := cacheImage.Save()
	c.Out.Printf("\n*** Cache Image: %s@%s\n", c.RepoName, sha)
	return err
}

func (c *LocalImageCacher) RequiresTar() bool {
	return true
}
