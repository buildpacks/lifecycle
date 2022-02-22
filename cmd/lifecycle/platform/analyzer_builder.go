package platform

import (
	"github.com/buildpacks/imgutil"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/platform"
)

type AnalyzerBuilder struct {
	PlatformAPI  *api.Version
	ImageHandler ImageHandler
}

type ImageHandler interface {
	InitImage(imageRef string) (imgutil.Image, error)
	Docker() client.CommonAPIClient
	Keychain() authn.Keychain
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

func (ab *AnalyzerBuilder) NewAnalyzer(opts AnalyzerOpts, logger lifecycle.Logger) (*lifecycle.Analyzer, error) {
	// TODO: validate registry here?

	buildpacks, err := ab.initBuildpacks(opts.LegacyGroup, opts.LegacyGroupPath)
	if err != nil {
		return nil, errors.Wrap(err, "reading buildpack group")
	}

	cacheStore, err := ab.initCache(opts.CacheImageRef, opts.LegacyCacheDir)
	if err != nil {
		return nil, errors.Wrap(err, "initializing cache")
	}

	previousImage, err := ab.initPrevious(opts.PreviousImageRef, opts.LaunchCacheDir)
	if err != nil {
		return nil, errors.Wrap(err, "initializing previous image")
	}

	runImage, err := ab.initRun(opts.RunImageRef)
	if err != nil {
		return nil, errors.Wrap(err, "initializing run image")
	}

	return &lifecycle.Analyzer{
		Platform: platform.NewPlatform(ab.PlatformAPI.String()),
		Logger:   logger,

		Buildpacks:            buildpacks,
		Cache:                 cacheStore,
		LayerMetadataRestorer: ab.initLayerMetadataRestorer(opts.LayersDir, opts.SkipLayers, logger),
		PreviousImage:         previousImage,
		RunImage:              runImage,
		SBOMRestorer:          ab.initSBOMRestorer(opts.LayersDir, logger),
	}, nil
}

func (ab *AnalyzerBuilder) initBuildpacks(group buildpack.Group, path string) ([]buildpack.GroupBuildpack, error) {
	if ab.PlatformAPI.AtLeast("0.7") {
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

func (ab *AnalyzerBuilder) initCache(cacheImageRef, cacheDir string) (lifecycle.Cache, error) {
	if ab.PlatformAPI.AtLeast("0.7") {
		return nil, nil
	}
	return initCache(cacheImageRef, cacheDir, ab.ImageHandler.Keychain())
}

func (ab *AnalyzerBuilder) initLayerMetadataRestorer(layersDir string, skipLayers bool, logger lifecycle.Logger) layer.MetadataRestorer {
	if ab.PlatformAPI.AtLeast("0.7") {
		return nil
	}
	return &layer.DefaultMetadataRestorer{
		LayersDir:  layersDir,
		SkipLayers: skipLayers,
		Logger:     logger,
	}
}

func (ab *AnalyzerBuilder) initPrevious(imageRef string, launchCacheDir string) (imgutil.Image, error) {
	if imageRef == "" {
		return nil, nil
	}
	image, err := ab.ImageHandler.InitImage(imageRef)
	if err != nil {
		return nil, err
	}
	if ab.ImageHandler.Docker() == nil || launchCacheDir == "" {
		return image, nil
	}

	volumeCache, err := cache.NewVolumeCache(launchCacheDir)
	if err != nil {
		return nil, errors.Wrap(err, "creating launch cache")
	}
	return cache.NewCachingImage(image, volumeCache), nil
}

func (ab *AnalyzerBuilder) initRun(imageRef string) (imgutil.Image, error) {
	return ab.ImageHandler.InitImage(imageRef)
}

func (ab *AnalyzerBuilder) initSBOMRestorer(layersDir string, logger lifecycle.Logger) layer.SBOMRestorer {
	if ab.PlatformAPI.LessThan("0.8") {
		return nil
	}
	return &layer.DefaultSBOMRestorer{
		LayersDir: layersDir,
		Logger:    logger,
	}
}
