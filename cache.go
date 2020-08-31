package lifecycle

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/launch"
)

func (e *Exporter) Cache(layersDir string, cacheStore Cache) error {
	var err error
	origMeta, err := cacheStore.RetrieveMetadata()
	if err != nil {
		return errors.Wrap(err, "metadata for previous cache")
	}
	meta := CacheMetadata{}

	for _, bp := range e.StackBuildpacks {
		snapshot := filepath.Join(layersDir, fmt.Sprintf("%s.tgz", launch.EscapeID(bp.ID)))

		layerName := "snapshot"
		layerID := fmt.Sprintf("%s:%s", bp.ID, layerName)
		lmd := BuildpackLayerMetadata{}

		origLayerMetadata := origMeta.MetadataForBuildpack(bp.ID).Layers[layerName]
		if lmd.SHA, err = e.addOrReuseSnapshotLayer(cacheStore, layerID, snapshot, origLayerMetadata.SHA); err != nil {
			e.Logger.Warnf("Failed to cache layer '%s': %s", layerID, err)
			continue
		}

		lmd.Cache = true
		lmd.Build = false //it will be exposed at build whether we like it or not
		lmd.Launch = false
		bpMD := BuildpackLayersMetadata{
			ID:      bp.ID,
			Version: bp.Version,
			Layers: map[string]BuildpackLayerMetadata{
				layerName: lmd,
			},
		}
		meta.Buildpacks = append(meta.Buildpacks, bpMD)
	}

	for _, bp := range e.Buildpacks {
		bpDir, err := readBuildpackLayersDir(layersDir, bp)
		if err != nil {
			return errors.Wrapf(err, "reading layers for buildpack '%s'", bp.ID)
		}

		bpMD := BuildpackLayersMetadata{
			ID:      bp.ID,
			Version: bp.Version,
			Layers:  map[string]BuildpackLayerMetadata{},
		}
		for _, layer := range bpDir.findLayers(forCached) {
			layer := layer
			if !layer.hasLocalContents() {
				e.Logger.Warnf("Failed to cache layer '%s' because it has no contents", layer.Identifier())
				continue
			}
			lmd, err := layer.read()
			if err != nil {
				e.Logger.Warnf("Failed to cache layer '%s' because of error reading metadata: %s", layer.Identifier(), err)
				continue
			}
			origLayerMetadata := origMeta.MetadataForBuildpack(bp.ID).Layers[layer.name()]
			if lmd.SHA, err = e.addOrReuseCacheLayer(cacheStore, &layer, origLayerMetadata.SHA); err != nil {
				e.Logger.Warnf("Failed to cache layer '%s': %s", layer.Identifier(), err)
				continue
			}
			bpMD.Layers[layer.name()] = lmd
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

func (e *Exporter) addOrReuseSnapshotLayer(cache Cache, id string, snapshotFile string, previousSHA string) (string, error) {
	layer, err := e.LayerFactory.SnapshotLayer(id, snapshotFile)
	if err != nil {
		return "", errors.Wrapf(err, "creating layer '%s'", id)
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

func (e *Exporter) addOrReuseCacheLayer(cache Cache, layerDir layerDir, previousSHA string) (string, error) {
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
