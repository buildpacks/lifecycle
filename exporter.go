package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/buildpack/lifecycle/image"
	"github.com/docker/docker/pkg/idtools"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/docker/docker/pkg/archive"
	"github.com/pkg/errors"
)

type Exporter struct {
	Buildpacks   []*Buildpack
	ArtifactsDir string
	In           []byte
	Out, Err     *log.Logger
	UID, GID     int
}

func (e *Exporter) Export(launchDirSrc, launchDirDst, appDirSrc, appDirDst string, runImage, origImage image.Image) error {
	metadata, err := e.PrepareExport(launchDirSrc, launchDirDst, appDirSrc, appDirDst)
	if err != nil {
		return errors.Wrapf(err, "prepare export")
	}
	return e.ExportImage(launchDirDst, appDirDst, runImage, origImage, metadata)
}

func (e *Exporter) PrepareExport(launchDirSrc, launchDirDst, appDirSrc, appDirDst string) (*AppImageMetadata, error) {
	var err error
	var metadata AppImageMetadata

	metadata.App.SHA, err = e.exportTar(appDirSrc, appDirDst)
	if err != nil {
		return nil, errors.Wrap(err, "exporting app layer tar")
	}
	metadata.Config.SHA, err = e.exportTar(filepath.Join(launchDirSrc, "config"), filepath.Join(launchDirDst, "config"))
	if err != nil {
		return nil, errors.Wrap(err, "exporting config layer tar")
	}

	for _, buildpack := range e.Buildpacks {
		bpMetadata := BuildpackMetadata{ID: buildpack.ID, Version: buildpack.Version, Layers: make(map[string]LayerMetadata)}
		tomls, err := filepath.Glob(filepath.Join(launchDirSrc, buildpack.EscapedID(), "*.toml"))
		if err != nil {
			return nil, errors.Wrapf(err, "finding layer tomls")
		}
		for _, tomlFile := range tomls {
			var bpLayer LayerMetadata
			if filepath.Base(tomlFile) == "launch.toml" {
				continue
			}
			dir := strings.TrimSuffix(tomlFile, ".toml")
			layerName := filepath.Base(dir)
			_, err := os.Stat(dir)
			if !os.IsNotExist(err) {
				bpLayer.SHA, err = e.exportTar(
					filepath.Join(launchDirSrc, buildpack.EscapedID(), layerName),
					filepath.Join(launchDirDst, buildpack.EscapedID(), layerName),
				)
				if err != nil {
					return nil, errors.Wrapf(err, "exporting tar for layer '%s/%s'", buildpack.ID, layerName)
				}
			}
			var metadata map[string]interface{}
			if _, err := toml.DecodeFile(tomlFile, &metadata); err != nil {
				return nil, errors.Wrapf(err, "read metadata for layer %s/%s", buildpack.ID, layerName)
			}
			bpLayer.Data = metadata
			bpMetadata.Layers[layerName] = bpLayer
		}
		metadata.Buildpacks = append(metadata.Buildpacks, bpMetadata)
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return nil, errors.Wrap(err, "marshal metadata")
	}
	err = ioutil.WriteFile(filepath.Join(e.ArtifactsDir, "metadata.json"), data, 0600)
	if err != nil {
		return nil, errors.Wrap(err, "write metadata")
	}

	return &metadata, nil
}

func (e *Exporter) ExportImage(launchDirDst, appDirDst string, runImage, origImage image.Image, metadata *AppImageMetadata) error {
	var err error
	metadata.RunImage.TopLayer, err = runImage.TopLayer()
	if err != nil {
		return errors.Wrap(err, "get run image top layer SHA")
	}
	metadata.RunImage.SHA, err = runImage.Digest()
	if err != nil {
		return errors.Wrap(err, "get run image digest")
	}

	origMetadata, err := e.GetMetadata(origImage)
	if err != nil {
		return errors.Wrap(err, "metadata for previous image")
	}

	appImage := runImage
	appImage.Rename(origImage.Name())

	e.Out.Printf("adding app layer with diffID '%s'\n", metadata.App.SHA)
	if err := appImage.AddLayer(filepath.Join(e.ArtifactsDir, strings.TrimPrefix(metadata.App.SHA, "sha256:")+".tar")); err != nil {
		return errors.Wrap(err, "add app layer")
	}

	e.Out.Printf("adding config layer with diffID '%s'\n", metadata.Config.SHA)
	if err := appImage.AddLayer(filepath.Join(e.ArtifactsDir, strings.TrimPrefix(metadata.Config.SHA, "sha256:")+".tar")); err != nil {
		return errors.Wrap(err, "add config layer")
	}

	for index, bp := range metadata.Buildpacks {
		var prevBP *BuildpackMetadata
		for _, pbp := range origMetadata.Buildpacks {
			if pbp.ID == bp.ID {
				prevBP = &pbp
			}
		}

		layerKeys := make([]string, 0, len(bp.Layers))
		for n, _ := range bp.Layers {
			layerKeys = append(layerKeys, n)
		}
		sort.Strings(layerKeys)

		for _, layerName := range layerKeys {
			layer := bp.Layers[layerName]
			if layer.SHA == "" {
				if prevBP == nil {
					return fmt.Errorf(
						"cannot reuse '%s/%s', previous image has no metadata for buildpack '%s'",
						bp.ID, layerName, bp.ID)
				}
				prevLayer := prevBP.Layers[layerName]
				if prevLayer.SHA == "" {
					return fmt.Errorf(
						"cannot reuse '%s/%s', previous image has no metadata for layer '%s/%s'",
						bp.ID, layerName, bp.ID, layerName)

				}

				e.Out.Printf("reusing layer '%s/%s' with diffID '%s'\n", bp.ID, layerName, prevLayer.SHA)
				if err := appImage.ReuseLayer(prevLayer.SHA); err != nil {
					return errors.Wrapf(err, "reuse layer '%s/%s' from previous image", bp.ID, layerName)
				}
				metadata.Buildpacks[index].Layers[layerName] = prevLayer
			} else {
				e.Out.Printf("adding layer '%s/%s' with diffID '%s'\n", bp.ID, layerName, layer.SHA)
				appImage.AddLayer(filepath.Join(e.ArtifactsDir, strings.TrimPrefix(layer.SHA, "sha256:")+".tar"))
			}
		}
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return errors.Wrap(err, "marshall metadata")
	}
	e.Out.Printf("setting metadata label '%s'\n", MetadataLabel)
	if err := appImage.SetLabel(MetadataLabel, string(data)); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}
	e.Out.Printf("setting env var '%s=%s'\n", EnvLaunchDir, launchDirDst)
	if err := appImage.SetEnv(EnvLaunchDir, launchDirDst); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}
	e.Out.Printf("setting env var '%s=%s'\n", EnvAppDir, appDirDst)
	if err := appImage.SetEnv(EnvAppDir, appDirDst); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}

	e.Out.Println("writing image")
	_, err = appImage.Save()
	return err
}

func (e *Exporter) GetMetadata(image image.Image) (AppImageMetadata, error) {
	var metadata AppImageMetadata
	found, err := image.Found()
	if err != nil {
		return metadata, errors.Wrap(err, "looking for image")
	}
	if found {
		label, err := image.Label(MetadataLabel)
		if err != nil {
			return metadata, errors.Wrap(err, "getting metadata")
		}
		if err := json.Unmarshal([]byte(label), &metadata); err != nil {
			return metadata, err
		}
	}
	return metadata, nil
}

func (e *Exporter) writeWithSHA(r io.Reader) (string, error) {
	hasher := sha256.New()

	f, err := ioutil.TempFile(e.ArtifactsDir, "tarfile")
	if err != nil {
		return "", err
	}
	defer f.Close()

	w := io.MultiWriter(hasher, f)

	if _, err := io.Copy(w, r); err != nil {
		return "", err
	}

	sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))

	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), filepath.Join(e.ArtifactsDir, sha+".tar")); err != nil {
		return "", err
	}

	return "sha256:" + sha, nil
}

func (e *Exporter) exportTar(sourceDir, destDir string) (string, error) {
	name := filepath.Base(sourceDir)
	tarOptions := &archive.TarOptions{
		IncludeFiles: []string{name},
		RebaseNames: map[string]string{
			name: destDir,
		},
	}
	if e.UID > 0 && e.GID > 0 {
		tarOptions.ChownOpts = &idtools.Identity{
			UID: e.UID,
			GID: e.GID,
		}
	}
	rc, err := archive.TarWithOptions(filepath.Dir(sourceDir), tarOptions)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	return e.writeWithSHA(rc)
}
