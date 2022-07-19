package platform

import (
	"errors"
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
	if buildpacksDir, err = absoluteIfNotEmpty(buildpacksDir); err != nil {
		return nil, err
	}
	if extensionsDir, err = absoluteIfNotEmpty(extensionsDir); err != nil {
		return nil, err
	}
	return &DirStore{buildpacksDir: buildpacksDir, extensionsDir: extensionsDir}, nil
}

func (s *DirStore) Lookup(kind, id, version string) (buildpack.BuildModule, error) {
	switch kind {
	case buildpack.KindBuildpack:
		if s.buildpacksDir == "" {
			return nil, errors.New("missing buildpacks directory")
		}
		descriptorPath := filepath.Join(s.buildpacksDir, launch.EscapeID(id), version, "buildpack.toml")
		return buildpack.ReadDescriptor(descriptorPath)
	case buildpack.KindExtension:
		if s.extensionsDir == "" {
			return nil, errors.New("missing extensions directory")
		}
		descriptorPath := filepath.Join(s.extensionsDir, launch.EscapeID(id), version, "extension.toml")
		return buildpack.ReadDescriptor(descriptorPath)
	default:
		return nil, fmt.Errorf("unknown module kind: %s", kind)
	}
}
