package lifecycle

import (
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/buildpacks/lifecycle/buildpack"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/platform"
)

type Restorer struct {
	LayersDir         string
	Buildpacks        []buildpack.GroupBuildpack
	Logger            Logger
	SkipLayers        bool
	LayerAnalyzer     LayerAnalyzer
	MetadataRetriever MetadataRetriever
	PlatformAPI       *api.Version
	LayersMetadata    platform.LayersMetadata
}

// Restore restores metadata for launch and cache layers into the layers directory and attempts to restore layer data for cache=true layers, removing the layer when unsuccessful.
// If a usable cache is not provided, Restore will not restore any cache=true layer metadata.
func (r *Restorer) Restore(cache Cache) error {
	var (
		cacheMetadata platform.CacheMetadata
		err           error
	)

	if r.analyzesLayers() {
		if cacheMetadata, err = r.LayerAnalyzer.Analyze(r.Buildpacks, r.SkipLayers, r.LayersMetadata, cache); err != nil {
			return err
		}
	} else {
		// Create empty cache metadata in case a usable cache is not provided.
		cacheMetadata, err = r.MetadataRetriever.RetrieveFrom(cache)
		if err != nil {
			return err
		}
	}

	var g errgroup.Group
	for _, buildpack := range r.Buildpacks {
		buildpackDir, err := readBuildpackLayersDir(r.LayersDir, buildpack)
		if err != nil {
			return errors.Wrapf(err, "reading buildpack layer directory")
		}

		cachedLayers := cacheMetadata.MetadataForBuildpack(buildpack.ID).Layers
		for _, bpLayer := range buildpackDir.findLayers(forCached) {
			name := bpLayer.name()
			cachedLayer, exists := cachedLayers[name]
			if !exists {
				r.Logger.Infof("Removing %q, not in cache", bpLayer.Identifier())
				if err := bpLayer.remove(); err != nil {
					return errors.Wrapf(err, "removing layer")
				}
				continue
			}
			data, err := bpLayer.read()
			if err != nil {
				return errors.Wrapf(err, "reading layer")
			}
			if data.SHA != cachedLayer.SHA {
				r.Logger.Infof("Removing %q, wrong sha", bpLayer.Identifier())
				r.Logger.Debugf("Layer sha: %q, cache sha: %q", data.SHA, cachedLayer.SHA)
				if err := bpLayer.remove(); err != nil {
					return errors.Wrapf(err, "removing layer")
				}
			} else {
				r.Logger.Infof("Restoring data for %q from cache", bpLayer.Identifier())
				g.Go(func() error {
					return r.restoreLayer(cache, cachedLayer.SHA)
				})
			}
		}
	}
	if err := g.Wait(); err != nil {
		return errors.Wrap(err, "restoring data")
	}
	return nil
}

func (r *Restorer) restoreLayer(cache Cache, sha string) error {
	// Sanity check to prevent panic.
	if cache == nil {
		return errors.New("restoring layer: cache not provided")
	}
	r.Logger.Debugf("Retrieving data for %q", sha)
	rc, err := cache.RetrieveLayer(sha)
	if err != nil {
		return err
	}
	defer rc.Close()

	return layers.Extract(rc, "")
}

func (r *Restorer) analyzesLayers() bool {
	return r.PlatformAPI.Compare(api.MustParse("0.6")) >= 0
}
