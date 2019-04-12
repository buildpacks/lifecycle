package lifecycle

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/archive"
)

type Cacher struct {
	ArtifactsDir string
	Buildpacks   []*Buildpack
	Out, Err     *log.Logger
	UID, GID     int
}

func (c *Cacher) Cache(layersDir string, cache Cache) error {
	origMetadata, _, err := cache.RetrieveMetadata()
	if err != nil {
		return errors.Wrap(err, "metadata for previous cache")
	}

	newMetadata := CacheMetadata{
		Buildpacks: []BuildpackMetadata{},
	}

	for _, bp := range c.Buildpacks {
		bpDir, err := readBuildpackLayersDir(layersDir, *bp)
		if err != nil {
			return err
		}
		bpMetadata := BuildpackMetadata{
			ID:      bp.ID,
			Version: bp.Version,
			Layers:  map[string]LayerMetadata{},
		}
		for _, l := range bpDir.findLayers(cached) {
			if !l.hasLocalContents() {
				return fmt.Errorf("failed to cache layer '%s' because it has no contents", l.Identifier())
			}
			metadata, err := l.read()
			if err != nil {
				return err
			}
			origLayerMetadata := origMetadata.metadataForBuildpack(bp.ID).Layers[l.name()]
			if metadata.SHA, err = c.addOrReuseLayer(cache, l, origLayerMetadata.SHA); err != nil {
				return err
			}
			bpMetadata.Layers[l.name()] = metadata
		}
		newMetadata.Buildpacks = append(newMetadata.Buildpacks, bpMetadata)
	}

	if err := cache.SetMetadata(newMetadata); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}

	return cache.Commit()
}

func (c *Cacher) addOrReuseLayer(cache Cache, layer bpLayer, previousSHA string) (string, error) {
	tarPath := filepath.Join(c.ArtifactsDir, escapeIdentifier(layer.Identifier())+".tar")
	sha, err := archive.WriteTarFile(layer.Path(), tarPath, c.UID, c.GID)
	if err != nil {
		return "", errors.Wrapf(err, "caching layer '%s'", layer.Identifier())
	}

	if sha == previousSHA {
		return sha, cache.ReuseLayer(layer.Identifier(), previousSHA)
	}

	return sha, cache.AddLayer(layer.Identifier(), sha, tarPath)
}
