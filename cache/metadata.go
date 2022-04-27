package cache

import "github.com/buildpacks/lifecycle/buildpack"

type Metadata struct {
	BOM        LayerMetadata              `json:"sbom"`
	Buildpacks []buildpack.LayersMetadata `json:"buildpacks"`
}

type LayerMetadata struct {
	SHA string `json:"sha" toml:"sha"`
}

func (cm *Metadata) MetadataForBuildpack(id string) buildpack.LayersMetadata {
	for _, bpMD := range cm.Buildpacks {
		if bpMD.ID == id {
			return bpMD
		}
	}
	return buildpack.LayersMetadata{}
}
