package lifecycle

import (
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/launch"
)

type Buildpack struct {
	ID       string `toml:"id" json:"id"`
	Version  string `toml:"version" json:"version"`
	Optional bool   `toml:"optional,omitempty" json:"optional,omitempty"`
	API      string `toml:"api,omitempty" json:"-"`
	Homepage string `toml:"homepage,omitempty" json:"homepage,omitempty"`
}

func (bp Buildpack) String() string {
	return bp.ID + "@" + bp.Version
}

func (bp Buildpack) noOpt() Buildpack {
	bp.Optional = false
	return bp
}

func (bp Buildpack) noAPI() Buildpack {
	bp.API = ""
	return bp
}

func (bp Buildpack) noHomepage() Buildpack {
	bp.Homepage = ""
	return bp
}

func (bp Buildpack) Lookup(buildpacksDir string) (*DefaultBuildpackTOML, error) {
	bpTOML := DefaultBuildpackTOML{}
	bpPath, err := filepath.Abs(filepath.Join(buildpacksDir, launch.EscapeID(bp.ID), bp.Version))
	if err != nil {
		return nil, err
	}
	tomlPath := filepath.Join(bpPath, "buildpack.toml")
	if _, err := toml.DecodeFile(tomlPath, &bpTOML); err != nil {
		return nil, err
	}
	bpTOML.Path = bpPath
	return &bpTOML, nil
}

type BuildpackInfo struct {
	ID       string `toml:"id"`
	Version  string `toml:"version"`
	Name     string `toml:"name"`
	ClearEnv bool   `toml:"clear-env,omitempty"`
	Homepage string `toml:"homepage,omitempty"`
}
