package platform

import (
	"github.com/buildpacks/imgutil"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/platform"
)

type AnalyzerFactory struct {
	PlatformAPI       *api.Version
	ImageHandler      ImageHandler
	RegistryValidator RegistryValidator
}

//go:generate mockgen -package testmock -destination testmock/image_handler.go github.com/buildpacks/lifecycle/cmd/lifecycle/platform ImageHandler
type ImageHandler interface {
	InitImage(imageRef string) (imgutil.Image, error)
	Docker() bool
	Keychain() authn.Keychain
}

//go:generate mockgen -package testmock -destination testmock/registry_validator.go github.com/buildpacks/lifecycle/cmd/lifecycle/platform RegistryValidator
type RegistryValidator interface {
	ReadableRegistryImages() []string // TODO: come up with better function names
	WriteableRegistryImages() []string
}

// AnalyzerOpts holds the inputs needed to construct a new lifecycle.Analyzer.
// Fields are the cumulative total of inputs across all supported platform APIs.
type AnalyzerOpts struct {
	CacheImageRef    string
	LaunchCacheDir   string
	LayersDir        string
	LegacyCacheDir   string
	LegacyGroupPath  string
	PreviousImageRef string
	RunImageRef      string
	SkipLayers       bool

	LegacyGroup buildpack.Group // for creator
}

func (af *AnalyzerFactory) NewAnalyzer(opts AnalyzerOpts, logger lifecycle.Logger) (*lifecycle.Analyzer, error) {
	// TODO: validate registry here?

	buildpacks, err := af.initBuildpacks(opts.LegacyGroup, opts.LegacyGroupPath)
	if err != nil {
		return nil, errors.Wrap(err, "reading buildpack group")
	}

	cacheStore, err := af.initCache(opts.CacheImageRef, opts.LegacyCacheDir)
	if err != nil {
		return nil, errors.Wrap(err, "initializing cache")
	}

	previousImage, err := af.initPrevious(opts.PreviousImageRef, opts.LaunchCacheDir)
	if err != nil {
		return nil, errors.Wrap(err, "initializing previous image")
	}

	runImage, err := af.initRun(opts.RunImageRef)
	if err != nil {
		return nil, errors.Wrap(err, "initializing run image")
	}

	return &lifecycle.Analyzer{
		Platform: platform.NewPlatform(af.PlatformAPI.String()),
		Logger:   logger,

		Buildpacks:            buildpacks,
		Cache:                 cacheStore,
		LayerMetadataRestorer: af.initLayerMetadataRestorer(opts.LayersDir, opts.SkipLayers, logger),
		PreviousImage:         previousImage,
		RunImage:              runImage,
		SBOMRestorer:          af.initSBOMRestorer(opts.LayersDir, logger),
	}, nil
}

func (af *AnalyzerFactory) initBuildpacks(group buildpack.Group, path string) ([]buildpack.GroupBuildpack, error) {
	if af.PlatformAPI.AtLeast("0.7") {
		return nil, nil
	}
	if len(group.Group) > 0 {
		return group.Group, nil
	}
	group, err := buildpack.ReadGroup(path)
	if err != nil {
		return []buildpack.GroupBuildpack{}, err
	}
	// TODO: verify buildpack apis
	return group.Group, nil
}

func (af *AnalyzerFactory) initCache(cacheImageRef, cacheDir string) (lifecycle.Cache, error) {
	if af.PlatformAPI.AtLeast("0.7") {
		return nil, nil
	}
	if cacheImageRef == "" && cacheDir == "" {
		return nil, nil
	}
	return initCache(cacheImageRef, cacheDir, af.ImageHandler.Keychain())
}

func (af *AnalyzerFactory) initLayerMetadataRestorer(layersDir string, skipLayers bool, logger lifecycle.Logger) layer.MetadataRestorer {
	if af.PlatformAPI.AtLeast("0.7") {
		return nil
	}
	return &layer.DefaultMetadataRestorer{
		LayersDir:  layersDir,
		SkipLayers: skipLayers,
		Logger:     logger,
	}
}

func (af *AnalyzerFactory) initPrevious(imageRef string, launchCacheDir string) (imgutil.Image, error) {
	if imageRef == "" {
		return nil, nil
	}
	image, err := af.ImageHandler.InitImage(imageRef)
	if err != nil {
		return nil, err
	}
	if !af.ImageHandler.Docker() || launchCacheDir == "" {
		return image, nil
	}

	volumeCache, err := cache.NewVolumeCache(launchCacheDir)
	if err != nil {
		return nil, errors.Wrap(err, "creating launch cache")
	}
	return cache.NewCachingImage(image, volumeCache), nil
}

func (af *AnalyzerFactory) initRun(imageRef string) (imgutil.Image, error) {
	if af.PlatformAPI.LessThan("0.7") || imageRef == "" {
		return nil, nil
	}
	return af.ImageHandler.InitImage(imageRef)
}

func (af *AnalyzerFactory) initSBOMRestorer(layersDir string, logger lifecycle.Logger) layer.SBOMRestorer {
	if af.PlatformAPI.LessThan("0.8") {
		return nil
	}
	return &layer.DefaultSBOMRestorer{
		LayersDir: layersDir,
		Logger:    logger,
	}
}
