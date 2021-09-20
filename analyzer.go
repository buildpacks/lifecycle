package lifecycle

import (
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/dataformat"
)

type Platform interface {
	API() string
}

type Analyzer struct {
	Image    imgutil.Image
	Logger   Logger
	Platform Platform

	// Platform API < 0.7
	Buildpacks            []buildpack.GroupBuildpack
	Cache                 Cache
	LayerMetadataRestorer LayerMetadataRestorer
}

// Analyze fetches the layers metadata from the previous image and writes analyzed.toml.
func (a *Analyzer) Analyze() (dataformat.AnalyzedMetadata, error) {
	var (
		appMeta   dataformat.LayersMetadata
		cacheMeta platform.CacheMetadata
		imageID   *dataformat.ImageIdentifier
		err       error
	)

	if a.Image != nil { // Image is optional in Platform API >= 0.7
		imageID, err = a.getImageIdentifier(a.Image)
		if err != nil {
			return dataformat.AnalyzedMetadata{}, errors.Wrap(err, "retrieving image identifier")
		}

		// continue even if the label cannot be decoded
		if err := DecodeLabel(a.Image, dataformat.LayerMetadataLabel, &appMeta); err != nil {
			appMeta = dataformat.LayersMetadata{}
		}
	} else {
		appMeta = dataformat.LayersMetadata{}
	}

	if a.restoresLayerMetadata() {
		cacheMeta, err = retrieveCacheMetadata(a.Cache, a.Logger)
		if err != nil {
			return dataformat.AnalyzedMetadata{}, err
		}

		useShaFiles := true
		if err := a.LayerMetadataRestorer.Restore(a.Buildpacks, appMeta, cacheMeta, NewLayerSHAStore(useShaFiles)); err != nil {
			return dataformat.AnalyzedMetadata{}, err
		}
	}

	return dataformat.AnalyzedMetadata{
		Image:    imageID,
		Metadata: appMeta,
	}, nil
}

func (a *Analyzer) restoresLayerMetadata() bool {
	return api.MustParse(a.Platform.API()).LessThan("0.7")
}

func (a *Analyzer) getImageIdentifier(image imgutil.Image) (*dataformat.ImageIdentifier, error) {
	if !image.Found() {
		a.Logger.Infof("Previous image with name %q not found", image.Name())
		return nil, nil
	}
	identifier, err := image.Identifier()
	if err != nil {
		return nil, err
	}
	a.Logger.Debugf("Analyzing image %q", identifier.String())
	return &dataformat.ImageIdentifier{
		Reference: identifier.String(),
	}, nil
}
