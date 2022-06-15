package buildpack

const (
	DockerfileKindBuild = "build"
	DockerfileKindRun   = "run"
)

type Dockerfile struct {
	ExtensionID string `toml:"buildpack-id" json:"buildpackID"`
	Kind        string `toml:"kind"`
	Path        string `toml:"path"`
}
