package lifecycle

import (
	"fmt"
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/launch"
)

type Analyzer struct {
	BuildpacksDir string
	LayersDir     string
	Logger        Logger
	SkipLayers    bool
}

// Analyze restores metadata for launch and cache layers into the layers directory.
// If a usable cache is not provided, Analyze will not restore any cache=true layer metadata.
func (a *Analyzer) Analyze(image imgutil.Image, cache Cache) (AnalyzedMetadata, error) {
	imageID, err := a.getImageIdentifier(image)
	if err != nil {
		return AnalyzedMetadata{}, errors.Wrap(err, "retrieving image identifier")
	}

	var appMeta LayersMetadata
	// continue even if the label cannot be decoded
	if err := DecodeLabel(image, LayerMetadataLabel, &appMeta); err != nil {
		appMeta = LayersMetadata{}
	}

	var buildMeta BuildMetadata
	// continue even if the label cannot be decoded
	if err := DecodeLabel(image, BuildMetadataLabel, &buildMeta); err != nil {
		buildMeta = BuildMetadata{}
	}

	for _, bp := range appMeta.Buildpacks {
		if store := bp.Store; store != nil {
			if err := WriteTOML(filepath.Join(a.LayersDir, launch.EscapeID(bp.ID), "store.toml"), store); err != nil {
				return AnalyzedMetadata{}, err
			}
		}
	}

	if a.SkipLayers {
		a.Logger.Infof("Skipping buildpack layer analysis")
	} else if err := a.analyzeLayers(appMeta, buildMeta, cache); err != nil {
		return AnalyzedMetadata{}, err
	}

	return AnalyzedMetadata{
		Image:    imageID,
		Metadata: appMeta,
	}, nil
}

func (a *Analyzer) analyzeLayers(appMeta LayersMetadata, buildMeta BuildMetadata, cache Cache) error {
	// Create empty cache metadata in case a usable cache is not provided.
	var cacheMeta CacheMetadata
	if cache != nil {
		var err error
		if !cache.Exists() {
			a.Logger.Info("Layer cache not found")
		}
		cacheMeta, err = cache.RetrieveMetadata()
		if err != nil {
			return errors.Wrap(err, "retrieving cache metadata")
		}
	} else {
		a.Logger.Debug("Usable cache not provided, using empty cache metadata.")
	}

	for _, buildpack := range appMeta.Buildpacks {
		groupBuildpack, err := a.getGroupBuildpack(buildpack, buildMeta)
		if err != nil {
			return err
		}
		if groupBuildpack == nil {
			// a buildpack used in the previous build is not available in this build, so we'll skip its layers
			continue
		}

		buildpackDir, err := readBuildpackLayersDir(a.LayersDir, *groupBuildpack)
		if err != nil {
			return errors.Wrap(err, "reading buildpack layer directory")
		}

		// Restore metadata for launch=true layers.
		// The restorer step will restore the layer data for cache=true layers if possible or delete the layer.
		appLayers := buildpack.Layers
		for name, layer := range appLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, name)
			if !layer.Launch {
				a.Logger.Debugf("Not restoring metadata for %q, marked as launch=false", identifier)
				continue
			}
			if layer.Build && !layer.Cache {
				a.Logger.Debugf("Not restoring metadata for %q, marked as build=true, cache=false", identifier)
				continue
			}
			a.Logger.Infof("Restoring metadata for %q from app image", identifier)
			if err := a.writeLayerMetadata(buildpackDir, name, layer); err != nil {
				return err
			}
		}
	}

	for _, buildpack := range cacheMeta.Buildpacks {
		groupBuildpack := GroupBuildpack{
			ID:      buildpack.ID,
			Version: buildpack.Version,
		}

		buildpackDir, err := readBuildpackLayersDir(a.LayersDir, groupBuildpack)
		if err != nil {
			return errors.Wrap(err, "reading buildpack layer directory")
		}

		// Restore metadata for cache=true layers.
		// The restorer step will restore the layer data if possible or delete the layer.
		cachedLayers := buildpack.Layers
		for name, layer := range cachedLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, name)
			if !layer.Cache {
				a.Logger.Debugf("Not restoring %q from cache, marked as cache=false", identifier)
				continue
			}
			// If launch=true, the metadata was restored from the app image or the layer is stale.
			if layer.Launch {
				a.Logger.Debugf("Not restoring %q from cache, marked as launch=true", identifier)
				continue
			}
			a.Logger.Infof("Restoring metadata for %q from cache", identifier)
			if err := a.writeLayerMetadata(buildpackDir, name, layer); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Analyzer) getGroupBuildpack(buildpack BuildpackLayersMetadata, buildMeta BuildMetadata) (*GroupBuildpack, error) {
	for _, buildBuildpack := range buildMeta.Buildpacks {
		if buildBuildpack.ID == buildpack.ID {
			info, err := buildBuildpack.Lookup(a.BuildpacksDir)
			if err != nil {
				a.Logger.Warnf("Error reading buildpack directory for %s@%s", buildBuildpack.ID, buildBuildpack.Version)
				return &buildBuildpack, errors.Wrap(err, "reading buildpack directory")
			}
			buildBuildpack.API = info.API

			a.Logger.Warnf("Read buildpack directory for %s", buildBuildpack.ID)
			return &buildBuildpack, nil
		}
	}

	a.Logger.Warnf("Couldn't find buildpack directory for %s in %s", buildpack.ID, buildMeta.Buildpacks)
	return nil, nil
}

func (a *Analyzer) getImageIdentifier(image imgutil.Image) (*ImageIdentifier, error) {
	if !image.Found() {
		a.Logger.Infof("Previous image with name %q not found", image.Name())
		return nil, nil
	}
	identifier, err := image.Identifier()
	if err != nil {
		return nil, err
	}
	a.Logger.Debugf("Analyzing image %q", identifier.String())
	return &ImageIdentifier{
		Reference: identifier.String(),
	}, nil
}

func (a *Analyzer) writeLayerMetadata(buildpackDir bpLayersDir, name string, metadata BuildpackLayerMetadata) error {
	layer := buildpackDir.newBPLayer(name)
	a.Logger.Debugf("Writing layer metadata for %q", layer.Identifier())
	if err := layer.writeMetadata(metadata); err != nil {
		return err
	}
	return layer.writeSha(metadata.SHA)
}
