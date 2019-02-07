package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/fs"
	"github.com/buildpack/lifecycle/image"
)

type Exporter struct {
	Buildpacks   []*Buildpack
	ArtifactsDir string
	In           []byte
	Out, Err     *log.Logger
	UID, GID     int
}

func (e *Exporter) Export(layersDir, appDir string, runImage, origImage image.Image, launcher string) error {
	var err error
	metadata := AppImageMetadata{}
	metadata.RunImage.TopLayer, err = runImage.TopLayer()
	if err != nil {
		return errors.Wrap(err, "get run image top layer SHA")
	}
	metadata.RunImage.SHA, err = runImage.Digest()
	if err != nil {
		return errors.Wrap(err, "get run image digest")
	}

	origMetadata, err := getMetadata(origImage, e.Out)
	if err != nil {
		return errors.Wrap(err, "metadata for previous image")
	}

	runImage.Rename(origImage.Name())
	appImage := &loggingImage{
		Out:   e.Out,
		image: runImage,
	}

	metadata.App.SHA, err = e.addOrReuseLayer(appImage, &layer{path: appDir, identifier: "app"}, origMetadata.App.SHA)
	if err != nil {
		return errors.Wrap(err, "exporting app layer")
	}

	metadata.Config.SHA, err = e.addOrReuseLayer(appImage, &layer{path: filepath.Join(layersDir, "config"), identifier: "config"}, origMetadata.Config.SHA)
	if err != nil {
		return errors.Wrap(err, "exporting config layer")
	}

	metadata.Launcher.SHA, err = e.addOrReuseLayer(appImage, &layer{path: launcher, identifier: "launcher"}, origMetadata.Launcher.SHA)
	if err != nil {
		return errors.Wrap(err, "exporting launcher layer")
	}

	for _, bp := range e.Buildpacks {
		bpDir, err := readBuildpackLayersDir(layersDir, *bp)
		if err != nil {
			return errors.Wrapf(err, "reading layers for buildpack '%s'", bp.ID)
		}
		bpMD := BuildpackMetadata{ID: bp.ID, Layers: map[string]LayerMetadata{}}

		for _, layer := range bpDir.findLayers(launch) {
			lmd, err := layer.read()
			if err != nil {
				return errors.Wrapf(err, "reading '%s' metadata", layer.Identifier())
			}

			if layer.hasLocalContents() {
				origLayerMetadata := origMetadata.metadataForBuildpack(bp.ID).Layers[layer.name()]
				lmd.SHA, err = e.addOrReuseLayer(appImage, &layer, origLayerMetadata.SHA)
				if err != nil {
					return err
				}
			} else {
				if lmd.Cache {
					return fmt.Errorf("layer '%s' is cache=true but has no contents", layer.Identifier())
				}
				origLayerMetadata, ok := origMetadata.metadataForBuildpack(bp.ID).Layers[layer.name()]
				if !ok {
					return fmt.Errorf("cannot reuse '%s', previous image has no metadata for layer '%s'", layer.Identifier(), layer.Identifier())
				}

				if err := appImage.ReuseLayer(layer.Identifier(), origLayerMetadata.SHA); err != nil {
					return errors.Wrapf(err, "reusing layer: '%s'", layer.Identifier())
				}
				lmd.SHA = origLayerMetadata.SHA
			}
			bpMD.Layers[layer.name()] = lmd
		}

		if malformedLayers := bpDir.findLayers(malformed); len(malformedLayers) > 0 {
			ids := make([]string, 0, len(malformedLayers))
			for _, ml := range malformedLayers {
				ids = append(ids, ml.Identifier())
			}
			return fmt.Errorf("failed to parse metadata for layers '%s'", ids)
		}

		metadata.Buildpacks = append(metadata.Buildpacks, bpMD)
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return errors.Wrap(err, "marshall metadata")
	}
	if err := appImage.SetLabel(MetadataLabel, string(data)); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}
	if err := appImage.SetEnv(cmd.EnvLayersDir, layersDir); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}
	if err := appImage.SetEnv(cmd.EnvAppDir, appDir); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}
	if err := appImage.SetEntrypoint(launcher); err != nil {
		return errors.Wrap(err, "setting entrypoint")
	}
	if err := appImage.SetEmptyCmd(); err != nil {
		return errors.Wrap(err, "setting cmd")
	}
	_, err = appImage.Save()
	return err
}

func (e *Exporter) addOrReuseLayer(image *loggingImage, layer identifiableLayer, previousSha string) (string, error) {
	tarPath := filepath.Join(e.ArtifactsDir, escape(layer.Identifier())+".tar")
	sha, err := e.exportTar(layer.Path(), tarPath)
	if err != nil {
		return "", errors.Wrapf(err, "exporting layer '%s'", layer.Identifier())
	}
	if sha == previousSha {
		return sha, image.ReuseLayer(layer.Identifier(), previousSha)
	}
	return sha, image.AddLayer(layer.Identifier(), sha, tarPath)
}

func (e *Exporter) exportTar(sourceDir, dest string) (string, error) {
	hasher := sha256.New()
	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := io.MultiWriter(hasher, f)

	fs := &fs.FS{}
	err = fs.WriteTarArchive(w, sourceDir, e.UID, e.GID)
	if err != nil {
		return "", err
	}
	sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))
	return "sha256:" + sha, nil
}
