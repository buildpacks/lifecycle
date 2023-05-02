package cache

import (
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/platform/files"
)

type Metadata struct {
	BOM        files.LayerMetadata        `json:"sbom"`
	Buildpacks []buildpack.LayersMetadata `json:"buildpacks"`
}

// FIXME: TODO
func (cm *Metadata) MetadataForBuildpack(id string) buildpack.LayersMetadata {
	for _, bpMD := range cm.Buildpacks {
		if bpMD.ID == id {
			return bpMD
		}
	}
	return buildpack.LayersMetadata{}
}
