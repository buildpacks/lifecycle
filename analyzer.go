package lifecycle

import (
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/platform"
)

type Analyzer struct {
	Image    imgutil.Image
	Logger   Logger
	Platform cmd.Platform

	// Platform API < 0.7
	Buildpacks            []buildpack.GroupBuildpack
	Cache                 Cache
	LayerMetadataRestorer LayerMetadataRestorer
}

// Analyze fetches the layers metadata from the previous image and writes analyzed.toml.
func (a *Analyzer) Analyze() (platform.AnalyzedMetadata, error) {
	var (
		appMeta   platform.LayersMetadata
		cacheMeta platform.CacheMetadata
		imageID   *platform.ImageIdentifier
		err       error
	)

	if a.Image != nil {
		imageID, err = a.getImageIdentifier(a.Image)
		if err != nil {
			return platform.AnalyzedMetadata{}, errors.Wrap(err, "retrieving image identifier")
		}

		// continue even if the label cannot be decoded
		if err := DecodeLabel(a.Image, platform.LayerMetadataLabel, &appMeta); err != nil {
			appMeta = platform.LayersMetadata{}
		}
	} else {
		appMeta = platform.LayersMetadata{}
	}

	cacheMeta, err = retrieveCacheMetadata(a.Cache, a.Logger)
	if err != nil {
		return platform.AnalyzedMetadata{}, err
	}

	if a.restoresLayerMetadata() {
		if err := a.LayerMetadataRestorer.Restore(a.Buildpacks, appMeta, cacheMeta); err != nil {
			return platform.AnalyzedMetadata{}, err
		}
	}

	return platform.AnalyzedMetadata{
		Image:    imageID,
		Metadata: appMeta,
	}, nil
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

func (a *Analyzer) restoresLayerMetadata() bool {
	return api.MustParse(a.Platform.API()).Compare(api.MustParse("0.7")) < 0
}

func (a *Analyzer) getImageIdentifier(image imgutil.Image) (*platform.ImageIdentifier, error) {
	if !image.Found() {
		a.Logger.Infof("Previous image with name %q not found", image.Name())
		return nil, nil
	}
	identifier, err := image.Identifier()
	if err != nil {
		return nil, err
	}
	a.Logger.Debugf("Analyzing image %q", identifier.String())
	return &platform.ImageIdentifier{
		Reference: identifier.String(),
	}, nil
}
