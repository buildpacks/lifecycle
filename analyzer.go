package lifecycle

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/buildpack/lifecycle/image"
)

type Analyzer struct {
	Buildpacks []*Buildpack
	In         []byte
	Out, Err   *log.Logger
}

func (a *Analyzer) Analyze(image image.Image, launchDir string) error {
	found, err := image.Found()
	if err != nil {
		return err
	}

	if !found {
		a.Out.Printf("WARNING: skipping analyze, image '%s' not found or requires authentication to access\n", image.Name())
		return nil
	}
	metadata, err := a.getMetadata(image)
	if err != nil {
		return err
	}
	if metadata != nil {
		return a.analyze(launchDir, *metadata)
	}
	return nil
}

func (a *Analyzer) analyze(launchDir string, metadata AppImageMetadata) error {
	groupBPs := a.buildpacks()
	cachedBPs, err := cachedBuildpacks(launchDir)
	if err != nil {
		return err
	}

	for _, cachedBP := range cachedBPs {
		_, exists := groupBPs[cachedBP]
		if !exists {
			a.Out.Printf("removing cached layers for buildpack '%s' not in group\n", cachedBP)
			if err := os.RemoveAll(filepath.Join(launchDir, cachedBP)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}

	for groupBP := range groupBPs {
		bpDir := filepath.Join(launchDir, groupBP)
		cachedLayers, err := cachedLayers(bpDir)
		if err != nil {
			return err
		}

		var metadataBP BuildpackMetadata
		for i, mbp := range metadata.Buildpacks {
			if escape(mbp.ID) == groupBP {
				metadataBP = metadata.Buildpacks[i]
			}
		}

		for name, cachedLayer := range cachedLayers {
			if _, ok := metadataBP.Layers[name]; !ok && cachedLayer.Launch {
				a.Out.Printf("removing stale cached layer '%s/%s', not in metadata \n", metadataBP.ID, name)
				if err := a.removeLayer(bpDir, name); err != nil {
					return err
				}
			}
		}

		for name, layerMetadata := range metadataBP.Layers {
			if cachedLayer, ok := cachedLayers[name]; ok && cachedLayer.SHA != layerMetadata.SHA {
				a.Out.Printf("removing stale cached layer '%s/%s', sha '%s' does not match sha from metadata '%s' \n", metadataBP.ID, name, cachedLayer.SHA, layerMetadata.SHA)
				if err := a.removeLayer(bpDir, name); err != nil {
					return err
				}
			}

			if !layerMetadata.Build {
				a.Out.Printf("writing metadata for layer '%s/%s'\n", metadataBP.ID, name)
				path := filepath.Join(bpDir, name+".toml")
				if err := writeTOML(path, layerMetadata); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func cachedBuildpacks(launchDir string) ([]string, error) {
	cachedBps := make([]string, 0, 0)
	bpDirs, err := filepath.Glob(filepath.Join(launchDir, "*"))
	if err != nil {
		return nil, err
	}
	for _, dir := range bpDirs {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, err
		}
		if filepath.Base(dir) != "app" && info.IsDir() {
			cachedBps = append(cachedBps, filepath.Base(dir))
		}
	}
	return cachedBps, nil
}

func (a *Analyzer) removeLayer(buildpackDir, name string) error {
	if err := os.RemoveAll(filepath.Join(buildpackDir, name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(filepath.Join(buildpackDir, name+".sha")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(filepath.Join(buildpackDir, name+".toml")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func cachedLayers(buildpackDir string) (map[string]*LayerMetadata, error) {
	cachedLayers := make(map[string]*LayerMetadata)
	layerTomls, err := filepath.Glob(filepath.Join(buildpackDir, "*.toml"))
	if err != nil {
		return nil, err
	}
	for _, toml := range layerTomls {
		metadata, err := readTOML(toml)
		if err != nil {
			return nil, err
		}
		name := strings.TrimRight(filepath.Base(toml), ".toml")
		cachedLayers[name] = metadata
		if sha, err := ioutil.ReadFile(filepath.Join(buildpackDir, name+".sha")); os.IsNotExist(err) {
		} else if err != nil {
			return nil, err
		} else {
			cachedLayers[name].SHA = string(sha)
		}
	}
	return cachedLayers, nil
}

func (a *Analyzer) getMetadata(image image.Image) (*AppImageMetadata, error) {
	label, err := image.Label(MetadataLabel)
	if err != nil {
		return nil, err
	}
	if label == "" {
		a.Out.Printf("WARNING: skipping analyze, previous image metadata was not found\n")
		return nil, nil
	}

	metadata := &AppImageMetadata{}
	if err := json.Unmarshal([]byte(label), metadata); err != nil {
		a.Out.Printf("WARNING: skipping analyze, previous image metadata was incompatible\n")
		return nil, nil
	}
	return metadata, nil
}

func (a *Analyzer) buildpacks() map[string]struct{} {
	buildpacks := make(map[string]struct{}, len(a.Buildpacks))
	for _, b := range a.Buildpacks {
		buildpacks[b.EscapedID()] = struct{}{}
	}
	return buildpacks
}

func writeTOML(path string, metadata LayerMetadata) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	fh, err := os.Create(path)
	if err != nil {
		return err
	}

	defer fh.Close()
	return toml.NewEncoder(fh).Encode(metadata)
}

func readTOML(path string) (*LayerMetadata, error) {
	var metadata LayerMetadata
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	_, err = toml.DecodeFile(path, &metadata)
	if err != nil {
		return nil, err
	}

	return &metadata, err
}
