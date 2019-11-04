package lifecycle

import (
	"path"

	"github.com/buildpack/lifecycle/metadata"
)

const BuildMetadataLabel = "io.buildpacks.build.metadata"

type BuildMetadata struct {
	Processes  []Process        `toml:"processes" json:"processes"`
	Buildpacks []Buildpack      `toml:"buildpacks" json:"buildpacks"`
	BOM        []BOMEntry       `toml:"bom" json:"bom"`
	Launcher   LauncherMetadata `toml:"-" json:"launcher"`
}

type LauncherMetadata struct {
	Version string         `json:"version"`
	Source  SourceMetadata `json:"source"`
}

type SourceMetadata struct {
	Git GitMetadata `json:"git"`
}

type GitMetadata struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
}

func MetadataFilePath(layersDir string) string {
	return path.Join(layersDir, "config", "metadata.toml")
}

type CacheMetadata struct {
	Buildpacks []metadata.BuildpackLayersMetadata `json:"buildpacks"`
}

func (cm *CacheMetadata) MetadataForBuildpack(id string) metadata.BuildpackLayersMetadata {
	for _, bpMd := range cm.Buildpacks {
		if bpMd.ID == id {
			return bpMd
		}
	}
	return metadata.BuildpackLayersMetadata{}
}
