package lifecycle

import (
	"github.com/buildpacks/imgutil"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/image"
	"github.com/buildpacks/lifecycle/internal/fsutil"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
)

type AnalyzerFactory struct {
	platformAPI     *api.Version
	apiVerifier     BuildpackAPIVerifier
	cacheHandler    CacheHandler
	configHandler   ConfigHandler
	imageHandler    image.Handler
	registryHandler RegistryHandler
}

func NewAnalyzerFactory(
	platformAPI *api.Version,
	apiVerifier BuildpackAPIVerifier,
	cacheHandler CacheHandler,
	configHandler ConfigHandler,
	imageHandler image.Handler,
	registryHandler RegistryHandler,
) *AnalyzerFactory {
	return &AnalyzerFactory{
		platformAPI:     platformAPI,
		apiVerifier:     apiVerifier,
		cacheHandler:    cacheHandler,
		configHandler:   configHandler,
		imageHandler:    imageHandler,
		registryHandler: registryHandler,
	}
}

type Analyzer struct {
	PreviousImage imgutil.Image
	RunImage      imgutil.Image
	Logger        log.Logger
	SBOMRestorer  layer.SBOMRestorer
	PlatformAPI   *api.Version
}

// NewAnalyzer configures a new Analyzer according to the provided Platform API version.
func (f *AnalyzerFactory) NewAnalyzer(additionalTags []string, cacheImageRef string, launchCacheDir string, layersDir string, outputImageRef string, previousImageRef string, runImageRef string, skipLayers bool, logger log.Logger) (*Analyzer, error) {
	analyzer := &Analyzer{
		Logger:       logger,
		SBOMRestorer: &layer.NopSBOMRestorer{},
		PlatformAPI:  f.platformAPI,
	}

	if err := f.ensureRegistryAccess(additionalTags, cacheImageRef, outputImageRef, runImageRef, previousImageRef); err != nil {
		return nil, err
	}

	if f.platformAPI.AtLeast("0.8") && !skipLayers {
		analyzer.SBOMRestorer = &layer.DefaultSBOMRestorer{ // FIXME: eventually layer.NewSBOMRestorer should always return the default one, and then we can use the constructor
			LayersDir: layersDir,
			Logger:    logger,
		}
	}

	if err := f.setPrevious(analyzer, previousImageRef, launchCacheDir); err != nil {
		return nil, err
	}
	if err := f.setRun(analyzer, runImageRef); err != nil {
		return nil, err
	}
	return analyzer, nil
}

func (f *AnalyzerFactory) ensureRegistryAccess(
	additionalTags []string,
	cacheImageRef string,
	outputImageRef string,
	runImageRef string,
	previousImageRef string,
) error {
	var readImages, writeImages []string
	writeImages = append(writeImages, cacheImageRef)
	if f.imageHandler.Kind() == image.RemoteKind {
		readImages = append(readImages, previousImageRef, runImageRef)
		writeImages = append(writeImages, outputImageRef)
		writeImages = append(writeImages, additionalTags...)
	}

	if err := f.registryHandler.EnsureReadAccess(readImages...); err != nil {
		return errors.Wrap(err, "validating registry read access")
	}
	if err := f.registryHandler.EnsureWriteAccess(writeImages...); err != nil {
		return errors.Wrap(err, "validating registry write access")
	}
	return nil
}

func (f *AnalyzerFactory) setPrevious(analyzer *Analyzer, imageRef string, launchCacheDir string) error {
	if imageRef == "" {
		return nil
	}
	var err error
	analyzer.PreviousImage, err = f.imageHandler.InitImage(imageRef)
	if err != nil {
		return errors.Wrap(err, "getting previous image")
	}
	if launchCacheDir == "" || f.imageHandler.Kind() != image.LocalKind {
		return nil
	}

	volumeCache, err := cache.NewVolumeCache(launchCacheDir)
	if err != nil {
		return errors.Wrap(err, "creating launch cache")
	}
	analyzer.PreviousImage = cache.NewCachingImage(analyzer.PreviousImage, volumeCache)
	return nil
}

func (f *AnalyzerFactory) setRun(analyzer *Analyzer, imageRef string) error {
	if imageRef == "" {
		return nil
	}
	var err error
	analyzer.RunImage, err = f.imageHandler.InitImage(imageRef)
	if err != nil {
		return errors.Wrap(err, "getting run image")
	}
	return nil
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
				platform.GetTargetOSFromFileSystem(&fsutil.Detect{}, atm, a.Logger)
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
