package platform

import "github.com/buildpacks/lifecycle/platform/common"

type CacheMetadata struct {
	Buildpacks []common.BuildpackLayersMetadata `json:"buildpacks"`
}

func (cm *CacheMetadata) MetadataForBuildpack(id string) common.BuildpackLayersMetadata {
	for _, bpMD := range cm.Buildpacks {
		if bpMD.ID == id {
			return bpMD
		}
	}
	return common.BuildpackLayersMetadata{}
}
