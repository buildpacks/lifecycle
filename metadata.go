package lifecycle

import (
	"encoding/json"
	"log"

	"github.com/buildpack/lifecycle/image"
)

const (
	MetadataLabel = "io.buildpacks.lifecycle.metadata"
)

type AppImageMetadata struct {
	App        AppMetadata         `json:"app"`
	Config     ConfigMetadata      `json:"config"`
	Launcher   LauncherMetadata    `json:"launcher"`
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
	RunImage   RunImageMetadata    `json:"runImage"`
}

type AppMetadata struct {
	SHA string `json:"sha"`
}

type ConfigMetadata struct {
	SHA string `json:"sha"`
}

type LauncherMetadata struct {
	SHA string `json:"sha"`
}

type BuildpackMetadata struct {
	ID      string                   `json:"key"`
	Version string                   `json:"version"`
	Layers  map[string]LayerMetadata `json:"layers"`
}

type LayerMetadata struct {
	SHA    string      `json:"sha" toml:"-"`
	Data   interface{} `json:"data" toml:"metadata"`
	Build  bool        `json:"build" toml:"build"`
	Launch bool        `json:"launch" toml:"launch"`
	Cache  bool        `json:"cache" toml:"cache"`
}

type RunImageMetadata struct {
	TopLayer string `json:"topLayer"`
	SHA      string `json:"sha"`
}

func (m *AppImageMetadata) metadataForBuildpack(id string) BuildpackMetadata {
	for _, bpMd := range m.Buildpacks {
		if bpMd.ID == id {
			return bpMd
		}
	}
	return BuildpackMetadata{}
}

func getMetadata(image image.Image, log *log.Logger) (AppImageMetadata, error) {
	metadata := AppImageMetadata{}
	if found, err := image.Found(); err != nil {
		return metadata, err
	} else if !found {
		log.Printf("WARNING: image '%s' not found or requires authentication to access\n", image.Name())
		return metadata, nil
	}
	label, err := image.Label(MetadataLabel)
	if err != nil {
		return metadata, err
	}
	if label == "" {
		log.Printf("WARNING: image '%s' does not have '%s' label", image.Name(), MetadataLabel)
		return metadata, nil
	}

	if err := json.Unmarshal([]byte(label), &metadata); err != nil {
		log.Printf("WARNING: image '%s' has incompatible '%s' label\n", image.Name(), MetadataLabel)
		return metadata, nil
	}
	return metadata, nil
}
