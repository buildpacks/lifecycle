package packs

const (
	BuildLabel     = "sh.packs.build"
	BuildpackLabel = "sh.packs.buildpacks"
)

type BuildMetadata struct {
	App        AppMetadata         `json:"app"`
	Buildpacks []BuildpackMetadata `json:"buildpacks"`
	Stack      StackMetadata       `json:"stack"`
}

type BuildpackMetadata struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type AppMetadata struct {
	Name string `json:"name"`
	SHA  string `json:"sha"`
}

type StackMetadata struct {
	Name string `json:"name"`
	SHA  string `json:"sha"`
}
