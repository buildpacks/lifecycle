package lifecycle

import (
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type BuildpackMap map[string]*Buildpack

func NewBuildpackMap(dir string) (BuildpackMap, error) {
	buildpacks := BuildpackMap{}
	glob := filepath.Join(dir, "*", "*", "buildpack.toml")
	files, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}
	for _, bpTOML := range files {
		buildpackDir := filepath.Dir(bpTOML)
		base, version := filepath.Split(buildpackDir)
		_, id := filepath.Split(filepath.Clean(base))
		var buildpack Buildpack
		if _, err := toml.DecodeFile(bpTOML, &buildpack); err != nil {
			return nil, err
		}
		buildpack.Dir = buildpackDir
		buildpacks[id+"@"+version] = &buildpack
	}
	return buildpacks, nil
}

func (m BuildpackMap) FromList(l []string) []*Buildpack {
	var out []*Buildpack
	for _, ref := range l {
		if !strings.Contains(ref, "@") {
			ref += "@latest"
		}
		if bp, ok := m[ref]; ok {
			out = append(out, bp)
		}
	}
	return out
}

func (bg *BuildpackGroup) List() []string {
	var out []string
	for _, bp := range bg.Buildpacks {
		out = append(out, bp.ID+"@"+bp.Version)
	}
	return out
}
