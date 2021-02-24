package lifecycle

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/platform"
)

type LayerAnalyzer interface {
	RetrieveMetadataFrom(cache Cache) (platform.CacheMetadata, error)
	Analyze(buildpacks []buildpack.GroupBuildpack, skipLayers bool, appMeta platform.LayersMetadata, meta platform.CacheMetadata) error
}

type DefaultLayerAnalyzer struct {
	Logger    Logger
	LayersDir string
}

func NewLayerAnalyzer(logger Logger, layersDir string) LayerAnalyzer {
	return &DefaultLayerAnalyzer{
		LayersDir: layersDir,
		Logger:    logger,
	}
}

func (la *DefaultLayerAnalyzer) RetrieveMetadataFrom(cache Cache) (platform.CacheMetadata, error) {
	// Create empty cache metadata in case a usable cache is not provided.
	var cacheMeta platform.CacheMetadata
	if cache != nil {
		var err error
		if !cache.Exists() {
			la.Logger.Info("Layer cache not found")
		}
		cacheMeta, err = cache.RetrieveMetadata()
		if err != nil {
			return cacheMeta, errors.Wrap(err, "retrieving cache metadata")
		}
	} else {
		la.Logger.Debug("Usable cache not provided, using empty cache metadata.")
	}

	return cacheMeta, nil
}

func (la *DefaultLayerAnalyzer) Analyze(buildpacks []buildpack.GroupBuildpack, skipLayers bool, appMeta platform.LayersMetadata, meta platform.CacheMetadata) error {
	if err := la.restoreStoreTOML(appMeta, buildpacks); err != nil {
		return err
	}

	if err := la.analyzeLayers(appMeta, meta, skipLayers, buildpacks); err != nil {
		return err
	}

	return nil
}

func (la *DefaultLayerAnalyzer) restoreStoreTOML(appMeta platform.LayersMetadata, buildpacks []buildpack.GroupBuildpack) error {
	for _, bp := range buildpacks {
		if store := appMeta.MetadataForBuildpack(bp.ID).Store; store != nil {
			if err := WriteTOML(filepath.Join(la.LayersDir, launch.EscapeID(bp.ID), "store.toml"), store); err != nil {
				return err
			}
		}
	}
	return nil
}

func (la *DefaultLayerAnalyzer) analyzeLayers(appMeta platform.LayersMetadata, meta platform.CacheMetadata, skipLayers bool, buildpacks []buildpack.GroupBuildpack) error {
	if skipLayers {
		la.Logger.Infof("Skipping buildpack layer analysis")
		return nil
	}

	for _, buildpack := range buildpacks {
		buildpackDir, err := readBuildpackLayersDir(la.LayersDir, buildpack)
		if err != nil {
			return errors.Wrap(err, "reading buildpack layer directory")
		}

		// Restore metadata for launch=true layers.
		// The restorer step will restore the layer data for cache=true layers if possible or delete the layer.
		appLayers := appMeta.MetadataForBuildpack(buildpack.ID).Layers
		for name, layer := range appLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, name)
			if !layer.Launch {
				la.Logger.Debugf("Not restoring metadata for %q, marked as launch=false", identifier)
				continue
			}
			if layer.Build && !layer.Cache {
				la.Logger.Debugf("Not restoring metadata for %q, marked as build=true, cache=false", identifier)
				continue
			}
			la.Logger.Infof("Restoring metadata for %q from app image", identifier)
			if err := la.writeLayerMetadata(buildpackDir, name, layer); err != nil {
				return err
			}
		}

		// Restore metadata for cache=true layers.
		// The restorer step will restore the layer data if possible or delete the layer.
		cachedLayers := meta.MetadataForBuildpack(buildpack.ID).Layers
		for name, layer := range cachedLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, name)
			if !layer.Cache {
				la.Logger.Debugf("Not restoring %q from cache, marked as cache=false", identifier)
				continue
			}
			// If launch=true, the metadata was restored from the app image or the layer is stale.
			if layer.Launch {
				la.Logger.Debugf("Not restoring %q from cache, marked as launch=true", identifier)
				continue
			}
			la.Logger.Infof("Restoring metadata for %q from cache", identifier)
			if err := la.writeLayerMetadata(buildpackDir, name, layer); err != nil {
				return err
			}
		}
	}
	return nil
}

func (la *DefaultLayerAnalyzer) writeLayerMetadata(buildpackDir bpLayersDir, name string, metadata platform.BuildpackLayerMetadata) error {
	layer := buildpackDir.newBPLayer(name)
	la.Logger.Debugf("Writing layer metadata for %q", layer.Identifier())
	if err := layer.writeMetadata(metadata); err != nil {
		return err
	}
	return layer.writeSha(metadata.SHA)
}
