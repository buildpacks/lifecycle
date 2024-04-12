package phase

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/layout"
	"github.com/buildpacks/imgutil/layout/sparse"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/cmd"
	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/internal/layer"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
)

const kanikoDir = "/kaniko"

// Restorer TODO
type Restorer struct {
	LayersDir      string
	AnalyzedMD     files.Analyzed
	LayersMetadata files.LayersMetadata // deprecated, use AnalyzedMD instead
	Buildpacks     []buildpack.GroupElement
	Extensions     []buildpack.GroupElement
	UseLayout      bool
	// images
	BuilderImage imgutil.Image
	RunImage     imgutil.Image
	// services
	Cache                 Cache
	LayerMetadataRestorer layer.MetadataRestorer
	SBOMRestorer          layer.SBOMRestorer
	// common
	Logger      log.Logger
	PlatformAPI *api.Version
}

// NewRestorer configures a new Restorer according to the provided Platform API version.
func (f *ConnectedFactory) NewRestorer(inputs platform.LifecycleInputs, logger log.Logger, withOptionalGroup buildpack.Group) (*Restorer, error) {
	cache, err := f.cacheHandler.InitCache(
		inputs.CacheImageRef,
		inputs.CacheDir,
		inputs.PlatformAPI.LessThan("0.13"),
	)
	if err != nil {
		return nil, err
	}
	restorer := &Restorer{
		LayersDir: inputs.LayersDir,
		UseLayout: inputs.UseLayout,
		Cache:     cache,
		LayerMetadataRestorer: layer.NewDefaultMetadataRestorer(
			inputs.LayersDir,
			inputs.SkipLayers,
			logger,
		),
		SBOMRestorer: layer.NewSBOMRestorer(
			layer.SBOMRestorerOpts{
				LayersDir: inputs.LayersDir,
				Nop:       inputs.SkipLayers,
				Logger:    logger,
			},
			inputs.PlatformAPI,
		),
		Logger:      logger,
		PlatformAPI: inputs.PlatformAPI,
	}

	if restorer.AnalyzedMD, err = f.getAnalyzed(inputs.AnalyzedPath, logger); err != nil {
		return nil, err
	}
	restorer.LayersMetadata = restorer.AnalyzedMD.LayersMetadata // for backwards compatibility with library callers that might expect LayersMetadata
	if restorer.Buildpacks, err = f.getBuildpacks(inputs.GroupPath, withOptionalGroup, logger); err != nil {
		return nil, err
	}
	if restorer.Extensions, err = f.getExtensions(inputs.GroupPath, logger); err != nil {
		return nil, err
	}

	if restorer.supportsBuildImageExtension() && inputs.BuildImageRef != "" {
		restorer.BuilderImage, err = f.imageHandler.InitRemoteImage(inputs.BuildImageRef)
		if err != nil || !restorer.BuilderImage.Found() {
			return nil, fmt.Errorf("failed to initialize builder image %s", inputs.BuildImageRef)
		}
	}
	if restorer.shouldPullRunImage() {
		restorer.RunImage, err = f.imageHandler.InitRemoteImage(restorer.AnalyzedMD.RunImageImage()) // FIXME: if we have a digest reference available in `Reference` (e.g., in the non-daemon case) we should use it)
		if err != nil || !restorer.RunImage.Found() {
			return nil, fmt.Errorf("failed to initialize run image %s", restorer.AnalyzedMD.RunImageImage())
		}
	} else if restorer.shouldUpdateAnalyzed() {
		restorer.RunImage, err = f.imageHandler.InitImage(restorer.AnalyzedMD.RunImageImage())
		if err != nil || !restorer.RunImage.Found() {
			return nil, fmt.Errorf("failed to initialize run image %s", restorer.AnalyzedMD.RunImageImage())
		}
	}

	return restorer, nil
}

func (r *Restorer) supportsBuildImageExtension() bool {
	return r.PlatformAPI.AtLeast("0.10")
}

// RestoreAnalyzed TODO
func (r *Restorer) RestoreAnalyzed() error {
	if r.BuilderImage != nil {
		r.Logger.Debugf("Pulling manifest and config for builder image %s...", r.BuilderImage.Name())
		if err := r.pullSparse(r.BuilderImage); err != nil {
			return err
		}
		digestRef, err := r.BuilderImage.Identifier()
		if err != nil {
			return fmt.Errorf("failed to get digest reference for builder image %s", r.BuilderImage.Name())
		}
		r.AnalyzedMD.BuildImage = &files.ImageIdentifier{Reference: digestRef.String()}
		r.Logger.Debugf("Adding build image info to analyzed metadata: ")
		r.Logger.Debugf(encoding.ToJSONMaybe(r.AnalyzedMD.BuildImage))
	}
	if r.RunImage != nil {
		if r.shouldPullRunImage() {
			r.Logger.Debugf("Pulling manifest and config for run image %s...", r.RunImage.Name())
			if err := r.pullSparse(r.RunImage); err != nil {
				return err
			}
		}
		// update analyzed metadata, even if we only needed to pull the image, because
		// the extender needs a digest reference in analyzed.toml,
		// and daemon images will only have a daemon image ID
		if err := r.updateAnalyzedMD(); err != nil {
			return cmd.FailErr(err, "update analyzed metadata")
		}
	}
	return nil
}

func (r *Restorer) shouldPullRunImage() bool {
	if r.PlatformAPI.LessThan("0.12") {
		return false
	}
	if r.AnalyzedMD.RunImage == nil {
		return false
	}
	return r.AnalyzedMD.RunImage.Extend
}

func (r *Restorer) pullSparse(image imgutil.Image) error {
	baseCacheDir := filepath.Join(kanikoDir, "cache", "base")
	if err := os.MkdirAll(baseCacheDir, 0750); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}
	// check for usable kaniko dir
	if _, err := os.Stat(kanikoDir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read kaniko directory: %w", err)
		}
		return nil
	}

	// save to disk
	h, err := image.UnderlyingImage().Digest()
	if err != nil {
		return fmt.Errorf("failed to get remote image digest: %w", err)
	}
	path := filepath.Join(baseCacheDir, h.String())
	r.Logger.Debugf("Saving image metadata to %s...", path)

	sparseImage, err := sparse.NewImage(
		path,
		image.UnderlyingImage(),
		layout.WithMediaTypes(imgutil.DefaultTypes),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize sparse image: %w", err)
	}
	if err = sparseImage.Save(); err != nil {
		return fmt.Errorf("failed to save sparse image: %w", err)
	}
	return nil
}

func (r *Restorer) shouldUpdateAnalyzed() bool {
	if r.PlatformAPI.LessThan("0.10") {
		return false
	}
	if len(r.Extensions) == 0 {
		return false
	}
	if r.AnalyzedMD.RunImage == nil {
		return false
	}
	return !isPopulated(r.AnalyzedMD.RunImage.TargetMetadata)
}

func isPopulated(metadata *files.TargetMetadata) bool {
	return metadata != nil && metadata.OS != ""
}

func (r *Restorer) updateAnalyzedMD() error {
	if r.PlatformAPI.LessThan("0.10") {
		return nil
	}
	if r.RunImage == nil {
		return nil
	}
	digestRef, err := r.RunImage.Identifier()
	if err != nil {
		return errors.New("failed to get digest reference for run image")
	}
	var targetData *files.TargetMetadata
	if r.PlatformAPI.AtLeast("0.12") {
		targetData, err = platform.GetTargetMetadata(r.RunImage)
		if err != nil {
			return errors.New("failed to read target data from run image")
		}
	}
	r.Logger.Debugf("Run image info in analyzed metadata was: ")
	r.Logger.Debugf(encoding.ToJSONMaybe(r.AnalyzedMD.RunImage))
	r.AnalyzedMD.RunImage.Reference = digestRef.String()
	r.AnalyzedMD.RunImage.TargetMetadata = targetData
	r.Logger.Debugf("Run image info in analyzed metadata is: ")
	r.Logger.Debugf(encoding.ToJSONMaybe(r.AnalyzedMD.RunImage))
	return nil
}

// RestoreCache TODO
func (r *Restorer) RestoreCache() error {
	defer log.NewMeasurement("Restorer", r.Logger)()
	cacheMeta, err := retrieveCacheMetadata(r.Cache, r.Logger)
	if err != nil {
		return err
	}

	if r.LayerMetadataRestorer == nil {
		r.LayerMetadataRestorer = layer.NewDefaultMetadataRestorer(r.LayersDir, false, r.Logger)
	}

	if r.SBOMRestorer == nil {
		r.SBOMRestorer = layer.NewSBOMRestorer(layer.SBOMRestorerOpts{
			LayersDir: r.LayersDir,
			Logger:    r.Logger,
			Nop:       false,
		}, r.PlatformAPI)
	}

	layerSHAStore := layer.NewSHAStore()
	r.Logger.Debug("Restoring Layer Metadata")
	layersMD := r.AnalyzedMD.LayersMetadata
	if len(layersMD.Buildpacks) == 0 {
		layersMD = r.LayersMetadata // for backwards compatibility with library callers that do not set AnalyzedMD
	}
	if err := r.LayerMetadataRestorer.Restore(r.Buildpacks, layersMD, cacheMeta, layerSHAStore); err != nil {
		return err
	}

	var g errgroup.Group
	for _, bp := range r.Buildpacks {
		cachedLayers := cacheMeta.MetadataForBuildpack(bp.ID).Layers

		// At this point in the build, <layer>.toml files never contain layer types information
		// (this information is added by buildpacks during the `build` phase).
		// The cache metadata is the only way to identify cache=true layers.
		cachedFn := func(l buildpack.Layer) bool {
			bpLayer, ok := cachedLayers[filepath.Base(l.Path())]
			return ok && bpLayer.Cache
		}

		r.Logger.Debugf("Reading Buildpack Layers directory %s", r.LayersDir)
		buildpackDir, err := buildpack.ReadLayersDir(r.LayersDir, bp, r.Logger)
		if err != nil {
			return errors.Wrapf(err, "reading buildpack layer directory")
		}
		foundLayers := buildpackDir.FindLayers(cachedFn)

		for _, bpLayer := range foundLayers {
			cachedLayer, exists := cachedLayers[bpLayer.Name()]
			if !exists {
				// This should be unreachable, as "find layers" uses the same cache metadata as the map
				r.Logger.Infof("Removing %q, not in cache", bpLayer.Identifier())
				if err := bpLayer.Remove(); err != nil {
					return errors.Wrapf(err, "removing layer")
				}
				continue
			}

			layerSha, err := layerSHAStore.Get(bp.ID, bpLayer)
			if err != nil {
				return err
			}

			if layerSha != cachedLayer.SHA {
				r.Logger.Infof("Removing %q, wrong sha", bpLayer.Identifier())
				r.Logger.Debugf("Layer sha: %q, cache sha: %q", layerSha, cachedLayer.SHA)
				if err := bpLayer.Remove(); err != nil {
					return errors.Wrapf(err, "removing layer")
				}
			} else {
				r.Logger.Infof("Restoring data for %q from cache", bpLayer.Identifier())
				g.Go(func() error {
					return r.restoreCacheLayer(r.Cache, cachedLayer.SHA)
				})
			}
		}
	}

	if r.PlatformAPI.AtLeast("0.8") {
		g.Go(func() error {
			if cacheMeta.BOM.SHA != "" {
				r.Logger.Infof("Restoring data for SBOM from cache")
				if err := r.SBOMRestorer.RestoreFromCache(r.Cache, cacheMeta.BOM.SHA); err != nil {
					return err
				}
			}
			return r.SBOMRestorer.RestoreToBuildpackLayers(r.Buildpacks)
		})
	}

	if err := g.Wait(); err != nil {
		return errors.Wrap(err, "restoring data")
	}

	return nil
}

// Restore restores metadata for launch and cache layers into the layers directory and attempts to restore layer data for cache=true layers, removing the layer when unsuccessful.
// If a usable cache is not provided, Restore will not restore any cache=true layer metadata.
//
// Deprecated: use RestoreCache instead.
func (r *Restorer) Restore(cache Cache) error {
	r.Cache = cache
	return r.RestoreCache()
}

func (r *Restorer) restoreCacheLayer(cache Cache, sha string) error {
	// Sanity check to prevent panic.
	if cache == nil {
		return errors.New("restoring layer: cache not provided")
	}
	r.Logger.Debugf("Retrieving data for %q", sha)
	rc, err := cache.RetrieveLayer(sha)
	if err != nil {
		return err
	}
	defer func() {
		_ = rc.Close()
	}()

	return layers.Extract(rc, "")
}

func retrieveCacheMetadata(fromCache Cache, logger log.Logger) (platform.CacheMetadata, error) {
	// Create empty cache metadata in case a usable cache is not provided.
	var cacheMeta platform.CacheMetadata
	if fromCache != nil {
		var err error
		if !fromCache.Exists() {
			logger.Info("Layer cache not found")
		}
		cacheMeta, err = fromCache.RetrieveMetadata()
		if err != nil {
			return cacheMeta, errors.Wrap(err, "retrieving cache metadata")
		}
	} else {
		logger.Debug("Usable cache not provided, using empty cache metadata")
	}

	return cacheMeta, nil
}
