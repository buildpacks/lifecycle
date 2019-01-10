package lifecycle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/buildpack/lifecycle/fs"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle/cmd"
	"github.com/buildpack/lifecycle/image"
)

type Exporter struct {
	Buildpacks   []*Buildpack
	ArtifactsDir string
	In           []byte
	Out, Err     *log.Logger
	UID, GID     int
}

func (e *Exporter) Export(layersDir, appDir string, runImage, origImage image.Image, launcher string, cacher Cacher) error {
	metadata, err := e.prepareExport(layersDir, appDir, launcher, cacher)
	if err != nil {
		return errors.Wrapf(err, "prepare export")
	}

	err = cacher.Export(metadata)
	if err != nil {
		return errors.Wrapf(err, "export cache")
	}

	return e.exportImage(layersDir, appDir, launcher, runImage, origImage, metadata)
}

func (e *Exporter) prepareExport(layersDir, appDir, launcher string, cacher Cacher) (*AppImageMetadata, error) {
	var err error
	var metadata AppImageMetadata

	metadata.App.SHA, _, err = e.exportTar(appDir)
	if err != nil {
		return nil, errors.Wrap(err, "exporting app layer tar")
	}
	metadata.Config.SHA, _, err = e.exportTar(filepath.Join(layersDir, "config"))
	if err != nil {
		return nil, errors.Wrap(err, "exporting config layer tar")
	}
	metadata.Launcher.SHA, _, err = e.exportTar(launcher)
	if err != nil {
		return nil, errors.Wrap(err, "exporting launcher layer tar")
	}

	for _, buildpack := range e.Buildpacks {
		bpMetadata := BuildpackMetadata{ID: buildpack.ID, Version: buildpack.Version, Layers: make(map[string]LayerMetadata)}
		tomls, err := filepath.Glob(filepath.Join(layersDir, buildpack.EscapedID(), "*.toml"))
		if err != nil {
			return nil, errors.Wrapf(err, "finding layer tomls")
		}
		for _, tomlFile := range tomls {
			var metadata LayerMetadata
			if filepath.Base(tomlFile) == "launch.toml" {
				continue
			}
			layerDir := strings.TrimSuffix(tomlFile, ".toml")
			layerName := filepath.Base(layerDir)
			_, err := os.Stat(layerDir)
			if _, err := toml.DecodeFile(tomlFile, &metadata); err != nil {
				return nil, errors.Wrapf(err, "read metadata for layer %s/%s", buildpack.ID, layerName)
			}
			if metadata.Launch {
				if !os.IsNotExist(err) {
					metadata.SHA, metadata.Tar, err = e.exportTar(layerDir)
					if err != nil {
						return nil, errors.Wrapf(err, "exporting tar for layer '%s/%s'", buildpack.ID, layerName)
					}
					if metadata.Cache {
						e.Out.Printf("caching launch layer '%s/%s' with sha '%s'\n", bpMetadata.ID, layerName, metadata.SHA)
						if err := ioutil.WriteFile(filepath.Join(layersDir, buildpack.EscapedID(), layerName+".sha"), []byte(metadata.SHA), 0777); err != nil {
							return nil, errors.Wrapf(err, "writing layer sha")
						}
					}
				} else {
					if err := os.Remove(tomlFile); err != nil {
						return nil, errors.Wrap(err, "removing toml for reused layer")
					}
				}
			}
			if !metadata.Cache {
				e.Out.Printf("removing uncached layer '%s/%s'\n", bpMetadata.ID, layerName)
				if err := os.RemoveAll(layerDir); err != nil && !os.IsNotExist(err) {
					return nil, errors.Wrap(err, "removing uncached layer")
				}
				if err := os.Remove(tomlFile); err != nil && !os.IsNotExist(err) {
					return nil, errors.Wrap(err, "removing toml for uncached layer")
				}
			} else if !metadata.Launch {
				e.Out.Printf("caching unexported layer '%s/%s'\n", bpMetadata.ID, layerName)
				if cacher.RequiresTar() {
					metadata.SHA, metadata.Tar, err = e.exportTar(layerDir)
					if err != nil {
						return nil, errors.Wrapf(err, "exporting tar for cache layer '%s/%s'", buildpack.ID, layerName)
					}
				}
			}
			bpMetadata.Layers[layerName] = metadata
		}
		metadata.Buildpacks = append(metadata.Buildpacks, bpMetadata)
	}

	fis, err := ioutil.ReadDir(layersDir)

OUTER:
	for _, fi := range fis {
		for _, buildpack := range e.Buildpacks {
			if fi.Name() == buildpack.EscapedID() {
				continue OUTER
			}
		}
		if err := os.RemoveAll(filepath.Join(layersDir, fi.Name())); err != nil {
			return nil, errors.Wrap(err, "failed to cleanup layers dir")
		}
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

func (e *Exporter) exportImage(layersDir, appDir, launcher string, runImage, origImage image.Image, metadata *AppImageMetadata) error {
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
	repoName := origImage.Name()
	appImage.Rename(repoName)

	e.Out.Printf("adding layer 'app' with diffID '%s'\n", metadata.App.SHA)
	if err := appImage.AddLayer(filepath.Join(e.ArtifactsDir, strings.TrimPrefix(metadata.App.SHA, "sha256:")+".tar")); err != nil {
		return errors.Wrap(err, "add app layer")
	}

	e.Out.Printf("adding layer 'config' with diffID '%s'\n", metadata.Config.SHA)
	if err := appImage.AddLayer(filepath.Join(e.ArtifactsDir, strings.TrimPrefix(metadata.Config.SHA, "sha256:")+".tar")); err != nil {
		return errors.Wrap(err, "add config layer")
	}

	if origMetadata.Launcher.SHA == metadata.Launcher.SHA {
		e.Out.Printf("reusing layer 'launcher' with diffID '%s'\n", metadata.Launcher.SHA)
		if err := appImage.ReuseLayer(origMetadata.Launcher.SHA); err != nil {
			return errors.Wrapf(err, "reuse launch layer from previous image")
		}
	} else {
		e.Out.Printf("adding layer 'launcher' with diffID '%s'\n", metadata.Launcher.SHA)
		if err := appImage.AddLayer(filepath.Join(e.ArtifactsDir, strings.TrimPrefix(metadata.Launcher.SHA, "sha256:")+".tar")); err != nil {
			return errors.Wrap(err, "add launcher layer")
		}
	}

	for index, bp := range metadata.Buildpacks {
		var prevBP *BuildpackMetadata
		for index, pbp := range origMetadata.Buildpacks {
			if pbp.ID == bp.ID {
				prevBP = &origMetadata.Buildpacks[index]
			}
		}

		layerKeys := make([]string, 0, len(bp.Layers))
		for n, _ := range bp.Layers {
			layerKeys = append(layerKeys, n)
		}
		sort.Strings(layerKeys)

		for _, layerName := range layerKeys {
			prevLayer := prevLayer(layerName, prevBP)
			layer := bp.Layers[layerName]

			if layer.Launch {
				if layer.SHA == "" || layer.SHA == prevLayer.SHA {
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
					if err := appImage.AddLayer(layer.Tar); err != nil {
						return err
					}
				}
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
	e.Out.Printf("setting env var '%s=%s'\n", cmd.EnvLayersDir, layersDir)
	if err := appImage.SetEnv(cmd.EnvLayersDir, layersDir); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}
	e.Out.Printf("setting env var '%s=%s'\n", cmd.EnvAppDir, appDir)
	if err := appImage.SetEnv(cmd.EnvAppDir, appDir); err != nil {
		return errors.Wrap(err, "set app image metadata label")
	}
	e.Out.Printf("setting entrypoint '%s'\n", launcher)
	if err := appImage.SetEntrypoint(launcher); err != nil {
		return errors.Wrap(err, "setting entrypoint")
	}
	e.Out.Println("setting empty cmd")
	if err := appImage.SetCmd(); err != nil {
		return errors.Wrap(err, "setting cmd")
	}

	e.Out.Println("writing image")
	sha, err := appImage.Save()
	e.Out.Printf("\n*** Image: %s@%s\n", repoName, sha)
	return err
}

func prevLayer(name string, prevBP *BuildpackMetadata) LayerMetadata {
	if prevBP == nil {
		return LayerMetadata{}
	}
	prevLayer := prevBP.Layers[name]
	if prevLayer.SHA == "" {
		return LayerMetadata{}
	}
	return prevLayer
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

func (e *Exporter) exportTar(sourceDir string) (string, string, error) {
	hasher := sha256.New()
	f, err := ioutil.TempFile(e.ArtifactsDir, "tarfile")
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	w := io.MultiWriter(hasher, f)

	fs := &fs.FS{}
	err = fs.WriteTarArchive(w, sourceDir, e.UID, e.GID)
	if err != nil {
		return "", "", err
	}
	sha := hex.EncodeToString(hasher.Sum(make([]byte, 0, hasher.Size())))

	if err := f.Close(); err != nil {
		return "", "", err
	}

	filename := filepath.Join(e.ArtifactsDir, sha+".tar")
	if err := os.Rename(f.Name(), filename); err != nil {
		return "", "", err
	}

	return "sha256:" + sha, filename, nil
}
