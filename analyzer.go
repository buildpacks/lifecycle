package lifecycle

import (
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/platform"
)

type Analyzer struct {
	Buildpacks  []buildpack.GroupBuildpack
	LayersDir   string
	Logger      Logger
	SkipLayers  bool
	PlatformAPI *api.Version
}

// Analyze restores metadata for launch and cache layers into the layers directory.
// If a usable cache is not provided, Analyze will not restore any cache=true layer metadata.
func (a *Analyzer) Analyze(image imgutil.Image, cache Cache) (platform.AnalyzedMetadata, error) {
	imageID, err := a.getImageIdentifier(image)
	if err != nil {
		return platform.AnalyzedMetadata{}, errors.Wrap(err, "retrieving image identifier")
	}

	var appMeta platform.LayersMetadata
	// continue even if the label cannot be decoded
	if err := DecodeLabel(image, platform.LayerMetadataLabel, &appMeta); err != nil {
		appMeta = platform.LayersMetadata{}
	}

	for _, bp := range a.Buildpacks {
		if store := appMeta.MetadataForBuildpack(bp.ID).Store; store != nil {
			if err := WriteTOML(filepath.Join(a.LayersDir, launch.EscapeID(bp.ID), "store.toml"), store); err != nil {
				return platform.AnalyzedMetadata{}, err
			}
		}
	}

	if a.PlatformAPI.Compare(api.MustParse("0.6")) < 0 { // platform API < 0.6
		restorer := Restorer{
			LayersDir:  a.LayersDir,
			Buildpacks: a.Buildpacks,
			Logger:     a.Logger,
			SkipLayers: a.SkipLayers,
		}

		if err := restorer.analyzeLayers(appMeta, cache); err != nil {
			return platform.AnalyzedMetadata{}, err
		}
	}

	return platform.AnalyzedMetadata{
		Image:    imageID,
		Metadata: appMeta,
	}, nil
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
