package buildpack

const (
	DockerfileKindBuild = "build"
	DockerfileKindRun   = "run"
)

type DockerfileInfo struct {
	ExtensionID string
	Kind        string
	Path        string
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
