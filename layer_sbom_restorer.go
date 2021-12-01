package lifecycle

import (
	"fmt"
	"github.com/buildpacks/imgutil/fakes"
	"github.com/buildpacks/lifecycle/layers"
	"github.com/pkg/errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/launch"
)

type LayerSBOMRestorer struct {
	layersDir string
	logger    Logger
}

func NewLayerSBOMRestorer(layersDir string, logger Logger) *LayerSBOMRestorer {
	return &LayerSBOMRestorer{
		layersDir: layersDir,
		logger:    logger,
	}
}

func (r *LayerSBOMRestorer) RestoreFromPrevious(image *fakes.Image, layerDigest string) error {
	// Sanity check to prevent panic.
	if image == nil {
		return errors.Errorf("restoring layer: previous image not found for %q", layerDigest)
	}
	r.logger.Debugf("Retrieving previous image layer for %q", layerDigest)

	rc, err := image.GetLayer(layerDigest)
	if err != nil {
		return err
	}
	defer rc.Close()

	return layers.Extract(rc, "")
}

func (r *LayerSBOMRestorer) RestoreFromCache(cache Cache, layerDigest string) error {
	// Sanity check to prevent panic.
	if cache == nil {
		return errors.New("restoring layer: cache not provided")
	}
	r.logger.Debugf("Retrieving data for %q", layerDigest)

	rc, err := cache.RetrieveLayer(layerDigest)
	if err != nil {
		return err
	}
	defer rc.Close()

	return layers.Extract(rc, "")
}

func (r *LayerSBOMRestorer) RestoreToBuildpackLayers(detectedBps []buildpack.GroupBuildpack) error {
	var (
		cacheDir  = filepath.Join(r.layersDir, "sbom", "cache")
		launchDir = filepath.Join(r.layersDir, "sbom", "launch")
	)
	defer os.RemoveAll(filepath.Join(r.layersDir, "sbom"))

	if err := filepath.Walk(cacheDir, r.restoreSBOMFunc(detectedBps, "cache")); err != nil {
		return err
	}

	return filepath.Walk(launchDir, r.restoreSBOMFunc(detectedBps, "launch"))
}

func (r *LayerSBOMRestorer) restoreSBOMFunc(detectedBps []buildpack.GroupBuildpack, bomType string) func(path string, info fs.FileInfo, err error) error {
	var bomRegex *regexp.Regexp

	if runtime.GOOS == "windows" {
		bomRegex = regexp.MustCompile(fmt.Sprintf(`%s\\(.+)\\(.+)\\(sbom.+json)`, bomType))
	} else {
		bomRegex = regexp.MustCompile(fmt.Sprintf(`%s/(.+)/(.+)/(sbom.+json)`, bomType))
	}

	return func(path string, info fs.FileInfo, err error) error {
		if info == nil || !info.Mode().IsRegular() {
			return nil
		}

		matches := bomRegex.FindStringSubmatch(path)
		if len(matches) != 4 {
			return nil
		}

		var (
			bpID      = matches[1]
			layerName = matches[2]
			fileName  = matches[3]
			dest      = filepath.Join(r.layersDir, bpID, fmt.Sprintf("%s.%s", layerName, fileName))
		)

		if !r.contains(detectedBps, bpID) {
			return nil
		}

		return Copy(path, dest)
	}
}

func (r *LayerSBOMRestorer) contains(detectedBps []buildpack.GroupBuildpack, id string) bool {
	for _, bp := range detectedBps {
		if launch.EscapeID(bp.ID) == id {
			return true
		}
	}
	return false
}
