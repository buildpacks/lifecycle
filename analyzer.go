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
	Image         imgutil.Image
	LayersDir     string
	Logger        Logger
	LayerAnalyzer LayerAnalyzer
	Platform      cmd.Platform
	Buildpacks    []buildpack.GroupBuildpack
	SkipLayers    bool

	// Platform API < 0.7
	Cache Cache
}

// Analyze fetches the layers metadata from the previous image and writes analyzed.toml
func (a *Analyzer) Analyze() (platform.AnalyzedMetadata, error) {
	var (
		appMeta platform.LayersMetadata
		imageID *platform.ImageIdentifier
		err     error
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

	if a.restoresAnalyzedLayers() {
		if _, err := a.LayerAnalyzer.Analyze(a.Buildpacks, a.SkipLayers, appMeta, a.Cache); err != nil {
			return platform.AnalyzedMetadata{}, err
		}
	}

	return platform.AnalyzedMetadata{
		Image:    imageID,
		Metadata: appMeta,
	}, nil
}

func (a *Analyzer) restoresAnalyzedLayers() bool {
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
