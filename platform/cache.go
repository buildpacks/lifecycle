package platform

type CacheMetadata struct {
	BOM        LayerMetadata             `json:"sbom"`
	Buildpacks []BuildpackLayersMetadata `json:"buildpacks"`
}

func (cm *CacheMetadata) MetadataForBuildpack(id string) BuildpackLayersMetadata {
	for _, bpMD := range cm.Buildpacks {
		if bpMD.ID == id {
			return bpMD
		}
	}
	return BuildpackLayersMetadata{}
}
