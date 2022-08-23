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

type ExtendConfig struct {
	Build ExtendBuildConfig `toml:"build"`
}

type ExtendBuildConfig struct {
	Args []ExtendArg `toml:"args"`
}

type ExtendArg struct {
	Name  string `toml:"name"`
	Value string `toml:"value"`
}
