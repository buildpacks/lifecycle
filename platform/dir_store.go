package platform

import (
	"path/filepath"

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
	descriptorPath := filepath.Join(s.buildpacksDir, launch.EscapeID(id), version, "buildpack.toml")
	return buildpack.ReadDescriptor(descriptorPath)
}

func (s *DirStore) LookupExt(id, version string) (buildpack.BuildModule, error) {
	descriptorPath := filepath.Join(s.extensionsDir, launch.EscapeID(id), version, "extension.toml")
	return buildpack.ReadDescriptor(descriptorPath)
}
