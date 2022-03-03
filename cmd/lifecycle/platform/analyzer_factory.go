package platform

import (
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cache"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/platform"
)

type AnalyzerFactory struct {
	PlatformAPI *api.Version
	AnalyzerOpsManager
}

//go:generate mockgen -package testmock -destination testmock/analyzer_ops_manager.go github.com/buildpacks/lifecycle/cmd/lifecycle/platform AnalyzerOpsManager
type AnalyzerOpsManager interface {
	EnsureRegistryAccess(opts AnalyzerOpts) AnalyzerOp
	WithBuildpacks(group buildpack.Group, path string) AnalyzerOp
	WithCache(cacheImageRef, cacheDir string) AnalyzerOp
	WithLayerMetadataRestorer(layersDir string, skipLayers bool, logger lifecycle.Logger) AnalyzerOp
	WithPrevious(imageRef string, launchCacheDir string) AnalyzerOp
	WithRun(imageRef string) AnalyzerOp
	WithSBOMRestorer(layersDir string, logger lifecycle.Logger) AnalyzerOp
}

type AnalyzerOp func(*lifecycle.Analyzer) error

func NewAnalyzerFactory(platformAPI *api.Version, docker client.CommonAPIClient, keychain authn.Keychain) *AnalyzerFactory {
	return &AnalyzerFactory{
		PlatformAPI: platformAPI,
		AnalyzerOpsManager: &DefaultAnalyzerOpsManager{
			CacheHandler:      NewCacheHandler(keychain),
			ImageHandler:      NewImageHandler(docker, keychain),
			RegistryValidator: NewRegistryValidator(keychain),
		},
	}
}

type DefaultAnalyzerOpsManager struct {
	CacheHandler      CacheHandler
	ImageHandler      ImageHandler
	RegistryValidator RegistryValidator
}

// AnalyzerOpts holds the inputs needed to construct a new lifecycle.Analyzer.
// Fields are the cumulative total of inputs across all supported platform APIs.
type AnalyzerOpts struct {
	AdditionalTags   []string
	CacheImageRef    string
	LaunchCacheDir   string
	LayersDir        string
	LegacyCacheDir   string
	LegacyGroupPath  string
	OutputImageRef   string
	PreviousImageRef string
	RunImageRef      string
	SkipLayers       bool

	LegacyGroup buildpack.Group // for creator
}

func (af *AnalyzerFactory) NewAnalyzer(opts AnalyzerOpts, logger lifecycle.Logger) (*lifecycle.Analyzer, error) {
	analyzer := &lifecycle.Analyzer{
		Platform: platform.NewPlatform(af.PlatformAPI.String()),
		Logger:   logger,
	}

	var ops []AnalyzerOp
	switch {
	case af.PlatformAPI.AtLeast("0.8"):
		ops = append(ops,
			af.EnsureRegistryAccess(opts),
			af.WithPrevious(opts.PreviousImageRef, opts.LaunchCacheDir),
			af.WithRun(opts.RunImageRef),
			af.WithSBOMRestorer(opts.LayersDir, logger),
		)
	case af.PlatformAPI.AtLeast("0.7"):
		ops = append(ops,
			af.EnsureRegistryAccess(opts),
			af.WithPrevious(opts.PreviousImageRef, opts.LaunchCacheDir),
			af.WithRun(opts.RunImageRef),
		)
	default:
		ops = append(ops,
			af.WithBuildpacks(opts.LegacyGroup, opts.LegacyGroupPath),
			af.WithCache(opts.CacheImageRef, opts.LegacyCacheDir),
			af.WithLayerMetadataRestorer(opts.LayersDir, opts.SkipLayers, logger),
			af.WithPrevious(opts.PreviousImageRef, opts.LaunchCacheDir),
		)
	}

	var err error
	for _, op := range ops {
		if err = op(analyzer); err != nil {
			return nil, errors.Wrap(err, "initializing analyzer")
		}
	}
	return analyzer, nil
}

func (om *DefaultAnalyzerOpsManager) EnsureRegistryAccess(opts AnalyzerOpts) AnalyzerOp {
	return func(_ *lifecycle.Analyzer) error {
		var readImages, writeImages []string
		writeImages = appendNotEmpty(writeImages, opts.CacheImageRef)
		if !om.ImageHandler.Docker() {
			readImages = appendNotEmpty(readImages, opts.PreviousImageRef, opts.RunImageRef)
			writeImages = appendNotEmpty(writeImages, opts.OutputImageRef)
			writeImages = appendNotEmpty(writeImages, opts.AdditionalTags...)
		}

		if err := om.RegistryValidator.ValidateReadAccess(readImages); err != nil {
			return errors.Wrap(err, "validating registry read access")
		}
		if err := om.RegistryValidator.ValidateWriteAccess(writeImages); err != nil {
			return errors.Wrap(err, "validating registry write access")
		}
		return nil
	}
}

func (om *DefaultAnalyzerOpsManager) WithBuildpacks(group buildpack.Group, path string) AnalyzerOp {
	return func(analyzer *lifecycle.Analyzer) error {
		if len(group.Group) > 0 {
			return nil
		}
		group, err := buildpack.ReadGroup(path)
		if err != nil {
			return err
		}
		if err := verifyBuildpackApis(group); err != nil {
			return err
		}
		analyzer.Buildpacks = group.Group
		return nil
	}
}

func (om *DefaultAnalyzerOpsManager) WithCache(cacheImageRef, cacheDir string) AnalyzerOp {
	return func(analyzer *lifecycle.Analyzer) error {
		var err error
		if cacheImageRef != "" {
			analyzer.Cache, err = om.CacheHandler.InitImageCache(cacheImageRef)
		}
		if cacheDir != "" {
			analyzer.Cache, err = om.CacheHandler.InitVolumeCache(cacheDir)
		}
		return err
	}
}

func (om *DefaultAnalyzerOpsManager) WithLayerMetadataRestorer(layersDir string, skipLayers bool, logger lifecycle.Logger) AnalyzerOp {
	return func(analyzer *lifecycle.Analyzer) error {
		analyzer.LayerMetadataRestorer = &layer.DefaultMetadataRestorer{
			LayersDir:  layersDir,
			SkipLayers: skipLayers,
			Logger:     logger,
		}
		return nil
	}
}

func (om *DefaultAnalyzerOpsManager) WithPrevious(imageRef string, launchCacheDir string) AnalyzerOp {
	return func(analyzer *lifecycle.Analyzer) error {
		if imageRef == "" {
			return nil
		}
		var err error
		analyzer.PreviousImage, err = om.ImageHandler.InitImage(imageRef)
		if err != nil {
			return err
		}
		if launchCacheDir == "" || !om.ImageHandler.Docker() {
			return nil
		}

		volumeCache, err := cache.NewVolumeCache(launchCacheDir)
		if err != nil {
			return errors.Wrap(err, "creating launch cache")
		}
		analyzer.PreviousImage = cache.NewCachingImage(analyzer.PreviousImage, volumeCache)
		return nil
	}
}

func (om *DefaultAnalyzerOpsManager) WithRun(imageRef string) AnalyzerOp {
	return func(analyzer *lifecycle.Analyzer) error {
		if imageRef == "" {
			return nil
		}
		var err error
		analyzer.RunImage, err = om.ImageHandler.InitImage(imageRef)
		return err
	}
}

func (om *DefaultAnalyzerOpsManager) WithSBOMRestorer(layersDir string, logger lifecycle.Logger) AnalyzerOp {
	return func(analyzer *lifecycle.Analyzer) error {
		analyzer.SBOMRestorer = &layer.DefaultSBOMRestorer{
			LayersDir: layersDir,
			Logger:    logger,
		}
		return nil
	}
}

func verifyBuildpackApis(group buildpack.Group) error {
	for _, bp := range group.Group {
		if bp.API == "" {
			// if this group was generated by this lifecycle bp.API should be set
			// but if for some reason it isn't default to 0.2
			bp.API = "0.2"
		}
		if err := cmd.VerifyBuildpackAPI(bp.String(), bp.API); err != nil {
			return err
		}
	}
	return nil
}
