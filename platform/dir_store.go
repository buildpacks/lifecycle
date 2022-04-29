package platform

import (
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/launch"
)

type DirStore struct {
	buildpacksDir string
	extensionsDir string
}

func NewDirStore(buildpacksDir string, extensionsDir string) (*DirStore, error) {
	var err error
	if buildpacksDir, err = filepath.Abs(buildpacksDir); err != nil {
		return nil, err
	}
	if extensionsDir, err = filepath.Abs(extensionsDir); err != nil {
		return nil, err
	}
	return &DirStore{buildpacksDir: buildpacksDir, extensionsDir: extensionsDir}, nil
}

func (s *DirStore) LookupBp(id, version string) (buildpack.BuildModule, error) {
	bpTOML := buildpack.Descriptor{}
	dirPath := filepath.Join(s.buildpacksDir, launch.EscapeID(id), version)
	if _, err := toml.DecodeFile(filepath.Join(dirPath, "buildpack.toml"), &bpTOML); err != nil {
		return nil, err
	}
	bpTOML.Dir = dirPath
	return &bpTOML, nil
}

func (s *DirStore) LookupExt(id, version string) (buildpack.BuildModule, error) {
	extTOML := buildpack.Descriptor{}
	dirPath := filepath.Join(s.extensionsDir, launch.EscapeID(id), version)
	if _, err := toml.DecodeFile(filepath.Join(dirPath, "extension.toml"), &extTOML); err != nil {
		return nil, err
	}
	extTOML.Dir = dirPath
	return &extTOML, nil
}
