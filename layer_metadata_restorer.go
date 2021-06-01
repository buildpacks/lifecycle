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
	Restore(buildpacks []buildpack.GroupBuildpack, appMeta platform.LayersMetadata, cacheMeta platform.CacheMetadata) error
	CacheIsValid(buildpackID, cachedLayerSha string, layer BpLayer) (bool, error)
}

type DefaultLayerMetadataRestorer struct {
	logger                Logger
	layersDir             string
	skipLayers            bool
	useShaFile            bool
	buildpacksToLayersSha buildpackLayersToSha
}

func NewLayerMetadataRestorer(logger Logger, layersDir string, skipLayers, useShaFile bool) LayerMetadataRestorer {
	return &DefaultLayerMetadataRestorer{
		logger:                logger,
		layersDir:             layersDir,
		skipLayers:            skipLayers,
		useShaFile:            useShaFile,
		buildpacksToLayersSha: buildpackLayersToSha{},
	}
}

func (la *DefaultLayerMetadataRestorer) Restore(buildpacks []buildpack.GroupBuildpack, appMeta platform.LayersMetadata, cacheMeta platform.CacheMetadata) error {
	if err := la.restoreStoreTOML(appMeta, buildpacks); err != nil {
		return err
	}

	if err := la.restoreLayerMetadata(appMeta, cacheMeta, buildpacks); err != nil {
		return err
	}

	return nil
}

func (la *DefaultLayerMetadataRestorer) restoreStoreTOML(appMeta platform.LayersMetadata, buildpacks []buildpack.GroupBuildpack) error {
	for _, bp := range buildpacks {
		if store := appMeta.MetadataForBuildpack(bp.ID).Store; store != nil {
			if err := WriteTOML(filepath.Join(la.layersDir, launch.EscapeID(bp.ID), "store.toml"), store); err != nil {
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

type buildpackLayersToSha struct {
	buildpacksToLayersShaMap map[string]layerToSha
}

func initializeBuildpackLayersToSha() buildpackLayersToSha {
	return buildpackLayersToSha{make(map[string]layerToSha)}
}

// if the layer exists for the buildpack ID, its SHA will be returned. Otherwise, an empty string will be returned.
func (bls *buildpackLayersToSha) getShaByBuildpackLayers(buildpackID, layerName string) string {
	if layerToSha, buildpackExists := bls.buildpacksToLayersShaMap[buildpackID]; buildpackExists {
		if sha, layerExists := layerToSha.layerToShaMap[layerName]; layerExists {
			return sha
		}
	}
	return ""
}

func (bls *buildpackLayersToSha) addLayerToMap(buildpackID, layerName, sha string) {
	_, exists := bls.buildpacksToLayersShaMap[buildpackID]
	if !exists {
		bls.buildpacksToLayersShaMap[buildpackID] = initializeLayerToSha()
	}
	bls.buildpacksToLayersShaMap[buildpackID].layerToShaMap[layerName] = sha
}

func (la *DefaultLayerMetadataRestorer) restoreLayerMetadata(appMeta platform.LayersMetadata, cacheMeta platform.CacheMetadata, buildpacks []buildpack.GroupBuildpack) error {
	if la.skipLayers {
		la.logger.Infof("Skipping buildpack layer analysis")
		return nil
	}

	if !la.useShaFile {
		la.buildpacksToLayersSha = initializeBuildpackLayersToSha()
	}

	for _, buildpack := range buildpacks {
		buildpackDir, err := readBuildpackLayersDir(la.layersDir, buildpack, la.logger)
		if err != nil {
			return errors.Wrap(err, "reading buildpack layer directory")
		}

		// Restore metadata for launch=true layers.
		// The restorer step will restore the layer data for cache=true layers if possible or delete the layer.
		appLayers := appMeta.MetadataForBuildpack(buildpack.ID).Layers
		cachedLayers := cacheMeta.MetadataForBuildpack(buildpack.ID).Layers
		for layerName, layer := range appLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, layerName)
			if !layer.Launch {
				la.logger.Debugf("Not restoring metadata for %q, marked as launch=false", identifier)
				continue
			}
			if layer.Build && !layer.Cache {
				// layer is launch=true, build=true. Because build=true, the layer contents must be present in the build container.
				// There is no reason to restore the metadata file, because the buildpack will always recreate the layer.
				la.logger.Debugf("Not restoring metadata for %q, marked as build=true, cache=false", identifier)
				continue
			}
			if layer.Cache {
				if cacheLayer, ok := cachedLayers[layerName]; !ok || !cacheLayer.Cache {
					// The layer is not cache=true in the cache metadata and will not be restored.
					// Do not write the metadata file so that it is clear to the buildpack that it needs to recreate the layer.
					// Although a launch=true (only) layer still needs a metadata file, the restorer will remove the file anyway when it does its cleanup (for buildpack apis < 0.6).
					la.logger.Debugf("Not restoring metadata for %q, marked as cache=true, but not found in cache", identifier)
					continue
				}
			}
			la.logger.Infof("Restoring metadata for %q from app image", identifier)
			if err := la.writeLayerMetadata(buildpackDir, layerName, layer, buildpack.ID); err != nil {
				return err
			}
		}

		// Restore metadata for cache=true layers.
		// The restorer step will restore the layer data if possible or delete the layer.
		for layerName, layer := range cachedLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, layerName)
			if !layer.Cache {
				la.logger.Debugf("Not restoring %q from cache, marked as cache=false", identifier)
				continue
			}
			// If launch=true, the metadata was restored from the app image or the layer is stale.
			if layer.Launch {
				la.logger.Debugf("Not restoring %q from cache, marked as launch=true", identifier)
				continue
			}
			la.logger.Infof("Restoring metadata for %q from cache", identifier)
			if err := la.writeLayerMetadata(buildpackDir, layerName, layer, buildpack.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (la *DefaultLayerMetadataRestorer) writeLayerMetadata(buildpackDir bpLayersDir, layerName string, metadata platform.BuildpackLayerMetadata, buildpackID string) error {
	layer := buildpackDir.newBPLayer(layerName, buildpackDir.buildpack.API, la.logger)
	la.logger.Debugf("Writing layer metadata for %q", layer.Identifier())
	if err := layer.writeMetadata(metadata.LayerMetadataFile); err != nil {
		return err
	}
	if la.useShaFile {
		return layer.writeSha(metadata.SHA)
	}
	la.buildpacksToLayersSha.addLayerToMap(buildpackID, layerName, metadata.SHA)
	return nil
}

func (la *DefaultLayerMetadataRestorer) CacheIsValid(buildpackID, cachedLayerSha string, layer BpLayer) (bool, error) {
	var layerSha string
	if la.useShaFile {
		data, err := layer.read(la.useShaFile)
		if err != nil {
			return false, errors.Wrapf(err, "reading layer")
		}
		layerSha = data.SHA
	} else {
		layerSha = la.buildpacksToLayersSha.getShaByBuildpackLayers(buildpackID, layer.name())
	}

	if layerSha != cachedLayerSha {
		la.logger.Infof("Removing %q, wrong sha", layer.Identifier())
		la.logger.Debugf("Layer sha: %q, cache sha: %q", layerSha, cachedLayerSha)
		if err := layer.remove(); err != nil {
			return false, errors.Wrapf(err, "removing layer")
		}
	} else {
		la.logger.Infof("Restoring data for %q from cache", layer.Identifier())
		return true, nil
	}

	return false, nil
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
