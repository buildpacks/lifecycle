package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/fs"
	"github.com/buildpack/lifecycle/image"
)

type Cacher struct {
	ArtifactsDir string
	Buildpacks   []*Buildpack
	Out, Err     *log.Logger
	UID, GID     int
}

func (c *Cacher) Cache(layersDir string, oldCacheImage, newCacheImage image.Image) error {
	loggingCacheImage := &loggingImage{
		Out:   c.Out,
		image: newCacheImage,
	}

	origMetadata, err := getMetadata(oldCacheImage, c.Out)
	if err != nil {
		return errors.Wrap(err, "metadata for previous image")
	}

	newMetadata := AppImageMetadata{
		Buildpacks: []BuildpackMetadata{},
	}

	for _, bp := range c.Buildpacks {
		bpDir, err := readBuildpackLayersDir(layersDir, *bp)
		if err != nil {
			return err
		}
		bpMetadata := BuildpackMetadata{
			ID:      bp.ID,
			Version: bp.Version,
			Layers:  map[string]LayerMetadata{},
		}
		for _, l := range bpDir.findLayers(cached) {
			if !l.hasLocalContents() {
				return fmt.Errorf("failed to cache layer '%s' because it has no contents", l.Identifier())
			}
			metadata, err := l.read()
			if err != nil {
				return err
			}
			origLayerMetadata := origMetadata.metadataForBuildpack(bp.ID).Layers[l.name()]
			if metadata.SHA, err = c.addOrReuseLayer(loggingCacheImage, l, origLayerMetadata.SHA); err != nil {
				return err
			}
			bpMetadata.Layers[l.name()] = metadata
		}
		newMetadata.Buildpacks = append(newMetadata.Buildpacks, bpMetadata)
	}
	data, err := json.Marshal(newMetadata)
	if err != nil {
		return errors.Wrap(err, "marshall metadata")
	}
	if err := loggingCacheImage.SetLabel(MetadataLabel, string(data)); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}
	_, err = loggingCacheImage.Save()
	return err
}

func (c *Cacher) addOrReuseLayer(image *loggingImage, layer bpLayer, previousSHA string) (string, error) {
	sha, err := c.exportTar(layer.Path())
	if err != nil {
		return "", errors.Wrapf(err, "exporting layer '%s'", layer.Identifier())
	}
	if sha == previousSHA {
		return sha, image.ReuseLayer(layer.Identifier(), previousSHA)
	}
	return sha, image.AddLayer(layer.Identifier(), sha, c.tarPath(sha))
}

func (c *Cacher) exportTar(sourceDir string) (string, error) {
	hasher := sha256.New()
	f, err := ioutil.TempFile(c.ArtifactsDir, "tarfile")
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := io.MultiWriter(hasher, f)

	fs := &fs.FS{}
	err = fs.WriteTarArchive(w, sourceDir, c.UID, c.GID)
	if err != nil {
		return "", err
	}
	sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))

	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), filepath.Join(c.ArtifactsDir, sha+".tar")); err != nil {
		return "", err
	}

	return "sha256:" + sha, nil
}

func (c *Cacher) tarPath(sha string) string {
	return filepath.Join(c.ArtifactsDir, strings.TrimPrefix(sha, "sha256:")+".tar")
}
