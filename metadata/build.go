package metadata

const BuildMetadataLabel = "io.buildpacks.build.metadata"

type BuildMetadata struct {
	BOM        map[string]map[string]interface{} `json:"bom"`
	Buildpacks []BuildpackMetadata               `json:"buildpacks"`
}

type BuildpackMetadata struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}
