package lifecycle

import (
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/layermetadata"
	"github.com/buildpacks/lifecycle/platform"
)

func (e *Exporter) Cache(layersDir string, cacheStore Cache) error {
	var err error
	if !cacheStore.Exists() {
		e.Logger.Info("Layer cache not found")
	}
	origMeta, err := cacheStore.RetrieveMetadata()
	if err != nil {
		return errors.Wrap(err, "metadata for previous cache")
	}
	meta := platform.CacheMetadata{}

	for _, bp := range e.Buildpacks {
		bpDir, err := layermetadata.ReadBuildpackLayersDir(layersDir, bp, e.Logger)
		if err != nil {
			return errors.Wrapf(err, "reading layers for buildpack '%s'", bp.ID)
		}

		bpMD := platform.BuildpackLayersMetadata{
			ID:      bp.ID,
			Version: bp.Version,
			Layers:  map[string]platform.BuildpackLayerMetadata{},
		}
		for _, layer := range bpDir.FindLayers(layermetadata.ForCached) {
			layer := layer
			if !layer.HasLocalContents() {
				e.Logger.Warnf("Failed to cache layer '%s' because it has no contents", layer.Identifier())
				continue
			}
			lmd, err := layer.Read()
			if err != nil {
				e.Logger.Warnf("Failed to cache layer '%s' because of error reading metadata: %s", layer.Identifier(), err)
				continue
			}
			origLayerMetadata := origMeta.MetadataForBuildpack(bp.ID).Layers[layer.Name()]
			if lmd.SHA, err = e.addOrReuseCacheLayer(cacheStore, &layer, origLayerMetadata.SHA); err != nil {
				e.Logger.Warnf("Failed to cache layer '%s': %s", layer.Identifier(), err)
				continue
			}
			bpMD.Layers[layer.Name()] = lmd
		}
		meta.Buildpacks = append(meta.Buildpacks, bpMD)
	}

	if err := cacheStore.SetMetadata(meta); err != nil {
		return errors.Wrap(err, "setting cache metadata")
	}
	if err := cacheStore.Commit(); err != nil {
		return errors.Wrap(err, "committing cache")
	}

	return nil
}

func (e *Exporter) addOrReuseCacheLayer(cache Cache, layerDir layermetadata.LayerDir, previousSHA string) (string, error) {
	layer, err := e.LayerFactory.DirLayer(layerDir.Identifier(), layerDir.Path())
	if err != nil {
		return "", errors.Wrapf(err, "creating layer '%s'", layerDir.Identifier())
	}
	if layer.Digest == previousSHA {
		e.Logger.Infof("Reusing cache layer '%s'\n", layer.ID)
		e.Logger.Debugf("Layer '%s' SHA: %s\n", layer.ID, layer.Digest)
		return layer.Digest, cache.ReuseLayer(previousSHA)
	}
	e.Logger.Infof("Adding cache layer '%s'\n", layer.ID)
	e.Logger.Debugf("Layer '%s' SHA: %s\n", layer.ID, layer.Digest)
	return layer.Digest, cache.AddLayerFile(layer.TarPath, layer.Digest)
}
