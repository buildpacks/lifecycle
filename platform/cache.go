package platform

import "github.com/buildpacks/lifecycle/platform/dataformat"

type CacheMetadata struct {
	Buildpacks []dataformat.BuildpackLayersMetadata `json:"buildpacks"`
}

func (cm *CacheMetadata) MetadataForBuildpack(id string) dataformat.BuildpackLayersMetadata {
	for _, bpMD := range cm.Buildpacks {
		if bpMD.ID == id {
			return bpMD
		}
	}
	return dataformat.BuildpackLayersMetadata{}
}
