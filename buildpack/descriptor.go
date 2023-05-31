package buildpack

const (
	KindBuildpack = "Buildpack"
	KindExtension = "Extension"
)

//go:generate mockgen -package testmock -destination ../testmock/component_descriptor.go github.com/buildpacks/lifecycle/buildpack Descriptor
type Descriptor interface {
	API() string
	Homepage() string
	TargetList() []TargetMetadata
}

// BaseInfo is information shared by both buildpacks and extensions.
type BaseInfo struct {
	ClearEnv bool   `toml:"clear-env,omitempty"`
	Homepage string `toml:"homepage,omitempty"`
	ID       string `toml:"id"`
	Name     string `toml:"name"`
	Version  string `toml:"version"`
}
