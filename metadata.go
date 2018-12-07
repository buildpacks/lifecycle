package lifecycle

const (
	MetadataLabel = "io.buildpacks.lifecycle.metadata"
)

type AppImageMetadata struct {
	App        AppMetadata         `json:"app"`
	Config     ConfigMetadata      `json:"config"`
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
	RunImage   RunImageMetadata    `json:"runImage"`
}

type AppMetadata struct {
	SHA string `json:"sha"`
}

type ConfigMetadata struct {
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
}

//this
type RunImageMetadata struct {
	TopLayer string `json:"topLayer"`
	SHA      string `json:"sha"`
}
