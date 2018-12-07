package lifecycle

import (
	"encoding/json"
	"fmt"
	"github.com/buildpack/lifecycle/image"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Analyzer struct {
	Buildpacks []*Buildpack
	In         []byte
	Out, Err   io.Writer
}

func (a *Analyzer) Analyze(image image.Image, launchDir string) error {
	found, err := image.Found()

	if err != nil {
		return err
	}

	if found {
		metadata, err := a.getMetadata(image)
		if err != nil {
			return err
		}
		if metadata != nil {
			return a.analyzeMetadata(launchDir, *metadata)
		}
		return nil
	} else {
		fmt.Fprintf(a.Out, "WARNING: skipping analyze, image '%s' not found or requires authentication to access\n", image.Name())
		return nil
	}
}

func (a *Analyzer) analyzeMetadata(launchDir string, config AppImageMetadata) error {
	buildpacks := a.buildpacks()
	for _, buildpack := range config.Buildpacks {
		if _, exist := buildpacks[buildpack.ID]; !exist {
			continue
		}
		for name, metadata := range buildpack.Layers {
			path := filepath.Join(launchDir, buildpack.ID, name+".toml")
			if err := writeTOML(path, metadata.Data); err != nil {
				return err
			}
		}
	}

	return nil
}

func (a *Analyzer) getMetadata(image image.Image) (*AppImageMetadata, error) {
	label, err := image.Label(MetadataLabel)
	if err != nil && strings.Contains(err.Error(), "does not exist") {
		fmt.Fprintf(a.Out, "WARNING: skipping analyze, previous image metadata was not found\n")
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	metadata := &AppImageMetadata{}
	if err := json.Unmarshal([]byte(label), metadata); err != nil {
		fmt.Fprintf(a.Out, "WARNING: skipping analyze, previous image metadata was incompatible\n")
		return nil, nil
	}
	return metadata, nil
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
