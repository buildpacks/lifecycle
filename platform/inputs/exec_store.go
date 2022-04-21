package inputs

import (
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/launch"
)

// TODO: is this needed?
type Buildpack interface {
	Build(bpPlan buildpack.Plan, config buildpack.BuildConfig, bpEnv buildpack.BuildEnv) (buildpack.BuildResult, error)
	ConfigFile() *buildpack.Descriptor
	Detect(config *buildpack.DetectConfig, bpEnv buildpack.BuildEnv) buildpack.DetectRun
}

type ExecStore struct {
	BuildpacksDir string
	ExtensionsDir string
}

func NewExecStore(buildpacksDir string, extensionsDir string) (*ExecStore, error) {
	var err error
	if buildpacksDir, err = filepath.Abs(buildpacksDir); err != nil {
		return nil, err
	}
	if extensionsDir, err = filepath.Abs(extensionsDir); err != nil {
		return nil, err
	}
	return &ExecStore{
		BuildpacksDir: buildpacksDir,
		ExtensionsDir: extensionsDir,
	}, nil
}

func (s *ExecStore) LookupBp(id, version string) (Buildpack, error) {
	bpTOML := buildpack.Descriptor{}
	dirPath := filepath.Join(s.BuildpacksDir, launch.EscapeID(id), version)
	tomlPath := filepath.Join(dirPath, "buildpack.toml")
	if _, err := toml.DecodeFile(tomlPath, &bpTOML); err != nil {
		return nil, err
	}
	bpTOML.Dir = dirPath
	return &bpTOML, nil
}

func (s *ExecStore) LookupExt(id, version string) (Buildpack, error) {
	extTOML := buildpack.Descriptor{}
	dirPath := filepath.Join(s.ExtensionsDir, launch.EscapeID(id), version)
	tomlPath := filepath.Join(dirPath, "extension.toml")
	if _, err := toml.DecodeFile(tomlPath, &extTOML); err != nil {
		return nil, err
	}
	extTOML.Dir = dirPath
	return &extTOML, nil
}
