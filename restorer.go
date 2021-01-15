package lifecycle

import (
	"fmt"
	"path/filepath"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/buildpacks/lifecycle/platform"
)

type Restorer struct {
	LayersDir  string
	Buildpacks []buildpack.GroupBuildpack
	Logger     Logger
	SkipLayers bool
}

// Restore attempts to restore layer data for cache=true layers, removing the layer when unsuccessful.
// If a usable cache is not provided, Restore will remove all cache=true layer metadata.
func (r *Restorer) Restore(cache Cache) error {
	// Create empty cache metadata in case a usable cache is not provided.
	var meta platform.CacheMetadata
	if cache != nil {
		var err error
		if !cache.Exists() {
			r.Logger.Info("Layer cache not found")
		}
		meta, err = cache.RetrieveMetadata()
		if err != nil {
			return errors.Wrapf(err, "retrieving cache metadata")
		}
	} else {
		r.Logger.Debug("Usable cache not provided, using empty cache metadata.")
	}

	var g errgroup.Group
	for _, buildpack := range r.Buildpacks {
		buildpackDir, err := readBuildpackLayersDir(r.LayersDir, buildpack)
		if err != nil {
			return errors.Wrapf(err, "reading buildpack layer directory")
		}

		cachedLayers := meta.MetadataForBuildpack(buildpack.ID).Layers
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

func (r *Restorer) restoreStoreTOML(appMeta platform.LayersMetadata) error {
	for _, bp := range r.Buildpacks {
		if store := appMeta.MetadataForBuildpack(bp.ID).Store; store != nil {
			if err := WriteTOML(filepath.Join(r.LayersDir, launch.EscapeID(bp.ID), "store.toml"), store); err != nil {
				return err
			}
		}
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

func (r *Restorer) analyzeLayers(appMeta platform.LayersMetadata, cache Cache) error {
	if r.SkipLayers {
		r.Logger.Infof("Skipping buildpack layer analysis")
		return nil
	}

	// Create empty cache metadata in case a usable cache is not provided.
	var cacheMeta platform.CacheMetadata
	if cache != nil {
		var err error
		if !cache.Exists() {
			r.Logger.Info("Layer cache not found")
		}
		cacheMeta, err = cache.RetrieveMetadata()
		if err != nil {
			return errors.Wrap(err, "retrieving cache metadata")
		}
	} else {
		r.Logger.Debug("Usable cache not provided, using empty cache metadata.")
	}

	for _, buildpack := range r.Buildpacks {
		buildpackDir, err := readBuildpackLayersDir(r.LayersDir, buildpack)
		if err != nil {
			return errors.Wrap(err, "reading buildpack layer directory")
		}

		// Restore metadata for launch=true layers.
		// The restorer step will restore the layer data for cache=true layers if possible or delete the layer.
		appLayers := appMeta.MetadataForBuildpack(buildpack.ID).Layers
		for name, layer := range appLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, name)
			if !layer.Launch {
				r.Logger.Debugf("Not restoring metadata for %q, marked as launch=false", identifier)
				continue
			}
			if layer.Build && !layer.Cache {
				r.Logger.Debugf("Not restoring metadata for %q, marked as build=true, cache=false", identifier)
				continue
			}
			r.Logger.Infof("Restoring metadata for %q from app image", identifier)
			if err := r.writeLayerMetadata(buildpackDir, name, layer); err != nil {
				return err
			}
		}

		// Restore metadata for cache=true layers.
		// The restorer step will restore the layer data if possible or delete the layer.
		cachedLayers := cacheMeta.MetadataForBuildpack(buildpack.ID).Layers
		for name, layer := range cachedLayers {
			identifier := fmt.Sprintf("%s:%s", buildpack.ID, name)
			if !layer.Cache {
				r.Logger.Debugf("Not restoring %q from cache, marked as cache=false", identifier)
				continue
			}
			// If launch=true, the metadata was restored from the app image or the layer is stale.
			if layer.Launch {
				r.Logger.Debugf("Not restoring %q from cache, marked as launch=true", identifier)
				continue
			}
			r.Logger.Infof("Restoring metadata for %q from cache", identifier)
			if err := r.writeLayerMetadata(buildpackDir, name, layer); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Restorer) writeLayerMetadata(buildpackDir bpLayersDir, name string, metadata platform.BuildpackLayerMetadata) error {
	layer := buildpackDir.newBPLayer(name)
	r.Logger.Debugf("Writing layer metadata for %q", layer.Identifier())
	if err := layer.writeMetadata(metadata); err != nil {
		return err
	}
	return layer.writeSha(metadata.SHA)
}
