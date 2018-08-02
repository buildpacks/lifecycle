package lifecycle

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"

	"github.com/buildpack/packs"
	"github.com/buildpack/packs/img"
)

type Exporter struct {
	Buildpacks []Buildpack
	TmpDir     string
	In         []byte
	Out, Err   io.Writer
}

func (e *Exporter) Export(launchDir string, stackImage, origImage v1.Image) (v1.Image, error) {
	stackDigest, err := stackImage.Digest()
	if err != nil {
		return nil, packs.FailErr(err, "stack digest")
	}
	metadata := packs.BuildMetadata{
		App:        packs.AppMetadata{},
		Buildpacks: []packs.BuildpackMetadata{},
		Stack: packs.StackMetadata{
			SHA: stackDigest.String(),
		},
	}

	repoImage, topLayerDigest, err := e.addDirAsLayer(stackImage, filepath.Join(e.TmpDir, "app.tgz"), filepath.Join(launchDir, "app"), "launch/app")
	if err != nil {
		return nil, packs.FailErr(err, "append droplet to stack")
	}
	metadata.App.SHA = topLayerDigest

	repoImage, topLayerDigest, err = e.addDirAsLayer(repoImage, filepath.Join(e.TmpDir, "config.tgz"), filepath.Join(launchDir, "config"), "launch/config")
	if err != nil {
		return nil, packs.FailErr(err, "append droplet to stack")
	}
	metadata.Config.SHA = topLayerDigest

	for _, buildpack := range e.Buildpacks {
		bpMetadata := packs.BuildpackMetadata{Key: buildpack.ID}
		repoImage, bpMetadata.Layers, err = e.addBuildpackLayer(buildpack.ID, launchDir, repoImage, origImage)
		if err != nil {
			return nil, packs.FailErr(err, "append layers")
		}
		metadata.Buildpacks = append(metadata.Buildpacks, bpMetadata)
	}

	buildJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, packs.FailErr(err, "get encode metadata")
	}
	repoImage, err = img.Label(repoImage, packs.BuildLabel, string(buildJSON))
	if err != nil {
		return nil, packs.FailErr(err, "set metdata label")
	}

	return repoImage, nil
}

func (e *Exporter) addBuildpackLayer(id, launchDir string, repoImage, origImage v1.Image) (v1.Image, map[string]packs.LayerMetadata, error) {
	metadata := make(map[string]packs.LayerMetadata)
	origLayers := make(map[string]packs.LayerMetadata)
	if origImage != nil {
		data, err := e.GetMetadata(origImage)
		if err != nil {
			return nil, nil, err
		}
		for _, bp := range data.Buildpacks {
			if bp.Key == id {
				origLayers = bp.Layers
			}
		}
	}

	layers, err := filepath.Glob(filepath.Join(launchDir, id, "*.toml"))
	if err != nil {
		return nil, nil, err
	}
	for _, layer := range layers {
		if filepath.Base(layer) == "launch.toml" {
			continue
		}
		var layerDiffID string
		dir := strings.TrimSuffix(layer, ".toml")
		layerName := filepath.Base(dir)
		dirInfo, err := os.Stat(dir)
		if os.IsNotExist(err) {
			if origLayers[layerName].SHA == "" {
				return nil, nil, fmt.Errorf("toml file layer expected, but no previous image data: %s/%s", id, layerName)
			}
			layerDiffID = origLayers[layerName].SHA
			hash, err := v1.NewHash(layerDiffID)
			if err != nil {
				return nil, nil, packs.FailErr(err, "parse hash", origLayers[layerName].SHA)
			}
			topLayer, err := origImage.LayerByDiffID(hash)
			if err != nil {
				return nil, nil, packs.FailErr(err, "find previous layer", id, layerName)
			}
			repoImage, err = mutate.AppendLayers(repoImage, topLayer)
			if err != nil {
				return nil, nil, packs.FailErr(err, "append layer from previous image", id, layerName)
			}
		} else if err != nil {
			return nil, nil, err
		} else if !dirInfo.IsDir() {
			return nil, nil, fmt.Errorf("expected %s to be a directory", dir)
		} else {
			tarFile := filepath.Join(e.TmpDir, fmt.Sprintf("layer.%s.%s.tgz", id, layerName))
			repoImage, layerDiffID, err = e.addDirAsLayer(repoImage, tarFile, dir, filepath.Join("launch", id, layerName))
			if err != nil {
				return nil, nil, packs.FailErr(err, "append dir as layer")
			}
		}
		var tomlData map[string]interface{}
		if _, err := toml.DecodeFile(layer, &tomlData); err != nil {
			return nil, nil, packs.FailErr(err, "read layer toml data")
		}
		metadata[layerName] = packs.LayerMetadata{SHA: layerDiffID, Data: tomlData}
	}
	return repoImage, metadata, nil
}

func (e *Exporter) GetMetadata(image v1.Image) (packs.BuildMetadata, error) {
	var metadata packs.BuildMetadata
	cfg, err := image.ConfigFile()
	if err != nil {
		return metadata, fmt.Errorf("read config: %s", err)
	}
	label := cfg.Config.Labels["sh.packs.build"]
	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		return metadata, fmt.Errorf("unmarshal: %s", err)
	}
	return metadata, nil
}

func (e *Exporter) addDirAsLayer(image v1.Image, tarFile, fsDir, tarDir string) (v1.Image, string, error) {
	if err := e.createTarFile(tarFile, fsDir, tarDir); err != nil {
		return nil, "", packs.FailErr(err, "tar", fsDir, "to", tarFile)
	}
	newImage, topLayer, err := img.Append(image, tarFile)
	if err != nil {
		return nil, "", packs.FailErr(err, "append droplet to stack")
	}
	diffid, err := topLayer.DiffID()
	if err != nil {
		return nil, "", packs.FailErr(err, "calculate layer diffid")
	}
	return newImage, diffid.String(), nil
}

func (e *Exporter) createTarFile(tarFile, fsDir, tarDir string) error {
	fh, err := os.Create(tarFile)
	if err != nil {
		return fmt.Errorf("create file for tar: %s", err)
	}
	defer fh.Close()
	gzw := gzip.NewWriter(fh)
	defer gzw.Close()
	tw := tar.NewWriter(gzw)
	defer tw.Close()

	return filepath.Walk(fsDir, func(file string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.Mode().IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(fsDir, file)
		if err != nil {
			return err
		}

		var header *tar.Header
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(file)
			if err != nil {
				return err
			}
			header, err = tar.FileInfoHeader(fi, target)
			if err != nil {
				return err
			}
		} else {
			header, err = tar.FileInfoHeader(fi, fi.Name())
			if err != nil {
				return err
			}
		}
		header.Name = filepath.Join(tarDir, relPath)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if fi.Mode().IsRegular() {
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
		}
		return nil
	})
}
