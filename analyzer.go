package lifecycle

import (
	"github.com/buildpacks/imgutil"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/platform"
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

type AnalyzeOperation func(a *Analyzer, analyzedMD *platform.AnalyzedMetadata) error

// Analyze fetches the layers metadata from the previous image and writes analyzed.toml.
func (a *Analyzer) Analyze(ops ...AnalyzeOperation) (platform.AnalyzedMetadata, error) {
	analyzedMD := &platform.AnalyzedMetadata{}

	for _, op := range ops {
		if err := op(a, analyzedMD); err != nil {
			return platform.AnalyzedMetadata{}, err
		}
	}

	return *analyzedMD, nil
}

func (a *Analyzer) GetImageIdentifier(image imgutil.Image) (*platform.ImageIdentifier, error) {
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
