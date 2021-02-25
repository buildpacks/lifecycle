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
	Analyze(buildpacks []buildpack.GroupBuildpack, skipLayers bool, appMeta platform.LayersMetadata, cache Cache) (platform.CacheMetadata, error)
}

type DefaultLayerAnalyzer struct {
	Logger            Logger
	LayersDir         string
	MetadataRetriever CacheMetadataRetriever
}

func NewLayerAnalyzer(logger Logger, metadataRetriever CacheMetadataRetriever, layersDir string) LayerAnalyzer {
	return &DefaultLayerAnalyzer{
		LayersDir:         layersDir,
		Logger:            logger,
		MetadataRetriever: metadataRetriever,
	}
}

func (la *DefaultLayerAnalyzer) Analyze(buildpacks []buildpack.GroupBuildpack, skipLayers bool, appMeta platform.LayersMetadata, cache Cache) (platform.CacheMetadata, error) {
	cacheMeta, err := la.MetadataRetriever.RetrieveFrom(cache)
	if err != nil {
		return platform.CacheMetadata{}, err
	}

	if err := la.restoreStoreTOML(appMeta, buildpacks); err != nil {
		return platform.CacheMetadata{}, err
	}

	if err := la.analyzeLayers(appMeta, cacheMeta, skipLayers, buildpacks); err != nil {
		return platform.CacheMetadata{}, err
	}

	return cacheMeta, nil
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
