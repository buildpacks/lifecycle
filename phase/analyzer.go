package phase

import (
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
)

// Analyzer reads metadata from the previous image (if it exists) and the run image,
// and additionally restores the SBOM layer from the previous image for use later in the build.
type Analyzer struct {
	// images
	PreviousImage imgutil.Image
	RunImage      imgutil.Image
	// services
	SBOMRestorer layer.SBOMRestorer
	// common
	Logger      log.Logger
	PlatformAPI *api.Version
}

// NewAnalyzer configures a new Analyzer according to the provided Platform API version.
func (f *ConnectedFactory) NewAnalyzer(inputs platform.LifecycleInputs, logger log.Logger) (*Analyzer, error) {
	analyzer := &Analyzer{
		Logger: logger,
		SBOMRestorer: layer.NewSBOMRestorer(
			layer.SBOMRestorerOpts{
				LayersDir: inputs.LayersDir,
				Nop:       inputs.SkipLayers,
				Logger:    logger,
			},
			f.platformAPI, // FIXME: this should probably be inputs.PlatformAPI, although they are the same
		),
		PlatformAPI: f.platformAPI, // FIXME: this should probably be inputs.PlatformAPI, although they are the same
	}

	if err := f.ensureRegistryAccess(inputs); err != nil {
		return nil, err
	}

	var err error
	if analyzer.PreviousImage, err = f.getPreviousImage(inputs.PreviousImageRef, inputs.LaunchCacheDir); err != nil {
		return nil, err
	}
	if analyzer.RunImage, err = f.getRunImage(inputs.RunImageRef); err != nil {
		return nil, err
	}
	return analyzer, nil
}

// Analyze fetches the layers metadata from the previous image and writes analyzed.toml.
func (a *Analyzer) Analyze() (files.Analyzed, error) {
	defer log.NewMeasurement("Analyzer", a.Logger)()
	var (
		err              error
		appMeta          files.LayersMetadata
		previousImageRef string
		runImageRef      string
	)
	appMeta, previousImageRef, err = a.retrieveAppMetadata()
	if err != nil {
		return files.Analyzed{}, err
	}

	if sha := bomSHA(appMeta); sha != "" {
		if err = a.SBOMRestorer.RestoreFromPrevious(a.PreviousImage, sha); err != nil {
			return files.Analyzed{}, errors.Wrap(err, "retrieving launch SBOM layer")
		}
	}

	var (
		atm          *files.TargetMetadata
		runImageName string
	)
	if a.RunImage != nil {
		runImageRef, err = a.getImageIdentifier(a.RunImage)
		if err != nil {
			return files.Analyzed{}, errors.Wrap(err, "identifying run image")
		}
		if a.PlatformAPI.AtLeast("0.12") {
			runImageName = a.RunImage.Name()
			atm, err = platform.GetTargetMetadata(a.RunImage)
			if err != nil {
				return files.Analyzed{}, errors.Wrap(err, "unpacking metadata from image")
			}
			if atm.OS == "" {
				return files.Analyzed{}, errors.New("failed to find OS in run image config")
			}
		}
	}

	return files.Analyzed{
		PreviousImage: &files.ImageIdentifier{
			Reference: previousImageRef,
		},
		RunImage: &files.RunImage{
			Reference:      runImageRef, // the image identifier, e.g. "s0m3d1g3st" (the image identifier) when exporting to a daemon, or "some.registry/some-repo@sha256:s0m3d1g3st" when exporting to a registry
			TargetMetadata: atm,
			Image:          runImageName, // the provided tag, e.g., "some.registry/some-repo:some-tag" if supported by the platform
		},
		LayersMetadata: appMeta,
	}, nil
}

func (a *Analyzer) getImageIdentifier(image imgutil.Image) (string, error) {
	if !image.Found() {
		a.Logger.Infof("Image with name %q not found", image.Name())
		return "", nil
	}
	identifier, err := image.Identifier()
	if err != nil {
		return "", err
	}
	a.Logger.Debugf("Found image with identifier %q", identifier.String())
	return identifier.String(), nil
}

func bomSHA(appMeta files.LayersMetadata) string {
	if appMeta.BOM == nil {
		return ""
	}
	return appMeta.BOM.SHA
}

func (a *Analyzer) retrieveAppMetadata() (files.LayersMetadata, string, error) {
	if a.PreviousImage == nil {
		return files.LayersMetadata{}, "", nil
	}
	previousImageRef, err := a.getImageIdentifier(a.PreviousImage)
	if err != nil {
		return files.LayersMetadata{}, "", errors.Wrap(err, "identifying previous image")
	}
	if a.PreviousImage.Found() && !a.PreviousImage.Valid() {
		a.Logger.Infof("Ignoring image %q because it was corrupt", a.PreviousImage.Name())
		return files.LayersMetadata{}, "", nil
	}

	var appMeta files.LayersMetadata
	// continue even if the label cannot be decoded
	if err = image.DecodeLabel(a.PreviousImage, platform.LifecycleMetadataLabel, &appMeta); err != nil {
		return files.LayersMetadata{}, "", nil
	}
	return appMeta, previousImageRef, nil
}
