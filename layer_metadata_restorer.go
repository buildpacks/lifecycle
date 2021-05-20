package lifecycle

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/platform"
)

//go:generate mockgen -package testmock -destination testmock/layer_metadata_restorer.go github.com/buildpacks/lifecycle LayerMetadataRestorer
type LayerMetadataRestorer interface {
	Restore(buildpacks []buildpack.GroupBuildpack, appMeta platform.LayersMetadata, cacheMeta platform.CacheMetadata, writeShaToFile bool) (BuildpackLayersToSha, error)
}

type DefaultLayerMetadataRestorer struct {
	Logger     Logger
	LayersDir  string
	SkipLayers bool
}

func NewLayerMetadataRestorer(logger Logger, layersDir string, skipLayers bool) LayerMetadataRestorer {
	return &DefaultLayerMetadataRestorer{
		Logger:     logger,
		LayersDir:  layersDir,
		SkipLayers: skipLayers,
	}
}

func (la *DefaultLayerMetadataRestorer) Restore(buildpacks []buildpack.GroupBuildpack, appMeta platform.LayersMetadata, cacheMeta platform.CacheMetadata, writeShaToFile bool) (BuildpackLayersToSha, error) {
	if err := la.restoreStoreTOML(appMeta, buildpacks); err != nil {
		return BuildpackLayersToSha{}, err
	}

	buildpackLayersToSha, err := la.restoreLayerMetadata(appMeta, cacheMeta, buildpacks, writeShaToFile)
	if err != nil {
		return BuildpackLayersToSha{}, err
	}

	return buildpackLayersToSha, nil
}

func (la *DefaultLayerMetadataRestorer) restoreStoreTOML(appMeta platform.LayersMetadata, buildpacks []buildpack.GroupBuildpack) error {
	for _, bp := range buildpacks {
		if store := appMeta.MetadataForBuildpack(bp.ID).Store; store != nil {
			if err := WriteTOML(filepath.Join(la.LayersDir, launch.EscapeID(bp.ID), "store.toml"), store); err != nil {
				return err
			}
		}
	}
	return nil
}

type layerToSha struct {
	layerToShaMap map[string]string
}

func initializeLayerToSha() layerToSha {
	return layerToSha{make(map[string]string)}
}

type BuildpackLayersToSha struct {
	buildpacksToLayersShaMap map[string]layerToSha
}

func initializeBuildpackLayersToSha() BuildpackLayersToSha {
	return BuildpackLayersToSha{make(map[string]layerToSha)}
}

// if the layer exists for the buildpack ID, its SHA will be returned. Otherwise, an empty string will be returned.
func (bls *BuildpackLayersToSha) getShaByBuildpackLayers(buildpackID, layerName string) string {
	if layerToSha, buildpackExists := bls.buildpacksToLayersShaMap[buildpackID]; buildpackExists {
		if sha, layerExists := layerToSha.layerToShaMap[layerName]; layerExists {
			return sha
		}
	}
	return ""
}

func (bls *BuildpackLayersToSha) addLayerToMap(buildpackID, layerName, sha string) {
	_, ok := bls.buildpacksToLayersShaMap[buildpackID]
	if !ok {
		bls.buildpacksToLayersShaMap[buildpackID] = initializeLayerToSha()
	}
	bls.buildpacksToLayersShaMap[buildpackID].layerToShaMap[layerName] = sha
}

func (la *DefaultLayerMetadataRestorer) restoreLayerMetadata(appMeta platform.LayersMetadata, cacheMeta platform.CacheMetadata, buildpacks []buildpack.GroupBuildpack, writeShaToFile bool) (BuildpackLayersToSha, error) {
	var buildpackLayersToSha BuildpackLayersToSha
	if !writeShaToFile {
		buildpackLayersToSha = initializeBuildpackLayersToSha()
	}

	if la.SkipLayers {
		la.Logger.Infof("Skipping buildpack layer analysis")
		return BuildpackLayersToSha{}, nil
	}

	for _, buildpack := range buildpacks {
		buildpackDir, err := readBuildpackLayersDir(la.LayersDir, buildpack, la.Logger)
		if err != nil {
			return BuildpackLayersToSha{}, errors.Wrap(err, "reading buildpack layer directory")
		}

		// Restore metadata for launch=true layers.
		// The restorer step will restore the layer data for cache=true layers if possible or delete the layer.
		appLayers := appMeta.MetadataForBuildpack(buildpack.ID).Layers
		cachedLayers := cacheMeta.MetadataForBuildpack(buildpack.ID).Layers
		for layerName, layer := range appLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, layerName)
			if !layer.Launch {
				la.Logger.Debugf("Not restoring metadata for %q, marked as launch=false", identifier)
				continue
			}
			if layer.Build && !layer.Cache {
				// layer is launch=true, build=true. Because build=true, the layer contents must be present in the build container.
				// There is no reason to restore the metadata file, because the buildpack will always recreate the layer.
				la.Logger.Debugf("Not restoring metadata for %q, marked as build=true, cache=false", identifier)
				continue
			}
			if layer.Cache {
				if cacheLayer, ok := cachedLayers[layerName]; !ok || !cacheLayer.Cache {
					// The layer is not cache=true in the cache metadata and will not be restored.
					// Do not write the metadata file so that it is clear to the buildpack that it needs to recreate the layer.
					// Although a launch=true (only) layer still needs a metadata file, the restorer will remove the file anyway when it does its cleanup (for buildpack apis < 0.6).
					la.Logger.Debugf("Not restoring metadata for %q, marked as cache=true, but not found in cache", identifier)
					continue
				}
			}
			la.Logger.Infof("Restoring metadata for %q from app image", identifier)
			if err := la.writeLayerMetadata(buildpackDir, layerName, layer, writeShaToFile, buildpack.ID, &buildpackLayersToSha); err != nil {
				return BuildpackLayersToSha{}, err
			}
		}

		// Restore metadata for cache=true layers.
		// The restorer step will restore the layer data if possible or delete the layer.
		for layerName, layer := range cachedLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, layerName)
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
			if err := la.writeLayerMetadata(buildpackDir, layerName, layer, writeShaToFile, buildpack.ID, &buildpackLayersToSha); err != nil {
				return BuildpackLayersToSha{}, err
			}
		}
	}
	return buildpackLayersToSha, nil
}

func (la *DefaultLayerMetadataRestorer) writeLayerMetadata(buildpackDir bpLayersDir, layerName string, metadata platform.BuildpackLayerMetadata, writeShaToFile bool, buildpackID string, buildpackLayersToSha *BuildpackLayersToSha) error {
	layer := buildpackDir.newBPLayer(layerName, buildpackDir.buildpack.API, la.Logger)
	la.Logger.Debugf("Writing layer metadata for %q", layer.Identifier())
	if err := layer.writeMetadata(metadata.LayerMetadataFile); err != nil {
		return err
	}
	if writeShaToFile {
		return layer.writeSha(metadata.SHA)
	}
	buildpackLayersToSha.addLayerToMap(buildpackID, layerName, metadata.SHA)
	return nil
}

func retrieveCacheMetadata(cache Cache, logger Logger) (platform.CacheMetadata, error) {
	// Create empty cache metadata in case a usable cache is not provided.
	var cacheMeta platform.CacheMetadata
	if cache != nil {
		var err error
		if !cache.Exists() {
			logger.Info("Layer cache not found")
		}
		cacheMeta, err = cache.RetrieveMetadata()
		if err != nil {
			return cacheMeta, errors.Wrap(err, "retrieving cache metadata")
		}
	} else {
		logger.Debug("Usable cache not provided, using empty cache metadata")
	}

	return cacheMeta, nil
}
