package lifecycle

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/packs"
	"github.com/google/go-containerregistry/pkg/v1"
)

type Analyzer struct {
	Buildpacks []*Buildpack
	In         []byte
	Out, Err   io.Writer
}

func (a *Analyzer) Analyze(launchDir string, image v1.Image) error {
	config, err := a.getBuildMetadata(image)
	if err != nil {
		return err
	}

	buildpacks := a.buildpacks()
	for _, buildpack := range config.Buildpacks {
		if _, exist := buildpacks[buildpack.Key]; !exist {
			continue
		}
		for name, metadata := range buildpack.Layers {
			path := filepath.Join(launchDir, buildpack.Key, name+".toml")
			if err := writeTOML(path, metadata.Data); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *Analyzer) getBuildMetadata(image v1.Image) (packs.BuildMetadata, error) {
	configFile, err := image.ConfigFile()
	if err != nil {
		return packs.BuildMetadata{}, err
	}
	jsonConfig := configFile.Config.Labels[packs.BuildLabel]
	if jsonConfig == "" {
		return packs.BuildMetadata{}, nil
	}

	config := packs.BuildMetadata{}
	if err := json.Unmarshal([]byte(jsonConfig), &config); err != nil {
		return packs.BuildMetadata{}, err
	}

	return config, err
}

func (a *Analyzer) buildpacks() map[string]struct{} {
	buildpacks := make(map[string]struct{}, len(a.Buildpacks))
	for _, b := range a.Buildpacks {
		buildpacks[b.ID] = struct{}{}
	}
	return buildpacks
}

func writeTOML(path string, data interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	fh, err := os.Create(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	return toml.NewEncoder(fh).Encode(data)
}
