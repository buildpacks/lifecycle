package platform

import (
	"fmt"
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

func (s *DirStore) Lookup(kind, id, version string) (buildpack.BuildModule, error) {
	switch kind {
	case buildpack.KindBuildpack:
		descriptorPath := filepath.Join(s.buildpacksDir, launch.EscapeID(id), version, "buildpack.toml")
		return buildpack.ReadDescriptor(descriptorPath)
	case buildpack.KindExtension:
		descriptorPath := filepath.Join(s.extensionsDir, launch.EscapeID(id), version, "extension.toml")
		return buildpack.ReadDescriptor(descriptorPath)
	default:
		return nil, fmt.Errorf("unknown module kind: %s", kind)
	}
}
