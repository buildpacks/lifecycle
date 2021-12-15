package buildpack

import (
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/launch"
)

// TODO: does store belong in this package?
type Buildable interface {
	Build(bpPlan Plan, config BuildConfig, bpEnv BuildEnv) (BuildResult, error)
	ConfigFile() *Descriptor
	Detect(config *DetectConfig, bpEnv BuildEnv) DetectRun
}

type BpStore struct {
	Dir string
}

func NewBuildpackStore(dir string) (*BpStore, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return &BpStore{Dir: dir}, nil
}

func (f *BpStore) Lookup(bpID, bpVersion string) (Buildable, error) {
	var bpTOML Descriptor
	bpPath := filepath.Join(f.Dir, launch.EscapeID(bpID), bpVersion)
	tomlPath := filepath.Join(bpPath, "buildpack.toml")
	if _, err := toml.DecodeFile(tomlPath, &bpTOML); err != nil {
		return nil, err
	}
	bpTOML.Dir = bpPath
	return &bpTOML, nil
}

type ExtensionStore struct {
	Dir string
}

func NewExtensionStore(dir string) (*ExtensionStore, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return &ExtensionStore{Dir: dir}, nil
}

func (f *ExtensionStore) Lookup(extID, extVersion string) (Buildable, error) {
	var extTOML Extension
	extPath := filepath.Join(f.Dir, launch.EscapeID(extID), extVersion)
	tomlPath := filepath.Join(extPath, "extension.toml")
	if _, err := toml.DecodeFile(tomlPath, &extTOML); err != nil {
		return nil, err
	}
	extTOML.Dir = extPath
	return &extTOML, nil
}
