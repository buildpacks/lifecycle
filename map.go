package lifecycle

import (
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/buildpack/packs"
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

func (m BuildpackMap) mapFull(l []*Buildpack) []*Buildpack {
	out := make([]*Buildpack, 0, len(l))
	for _, i := range l {
		ref := i.ID + "@" + i.Version
		if i.Version == "" {
			ref += "latest"
		}
		if bp, ok := m[ref]; ok {
			out = append(out, bp)
		}
	}
	return out
}

func (m BuildpackMap) ReadOrder(orderPath string) (BuildpackOrder, error) {
	var order struct {
		Groups BuildpackOrder `toml:"groups"`
	}
	if _, err := toml.DecodeFile(orderPath, &order); err != nil {
		return nil, packs.FailErr(err, "read buildpack order")
	}

	var groups BuildpackOrder
	for _, g := range order.Groups {
		groups = append(groups, BuildpackGroup{
			Repository: g.Repository,
			Buildpacks: m.mapFull(g.Buildpacks),
		})
	}
	return groups, nil
}

func (g *BuildpackGroup) Write(path string) error {
	buildpacks := make([]*SimpleBuildpack, 0, len(g.Buildpacks))
	for _, b := range g.Buildpacks {
		buildpacks = append(buildpacks, &SimpleBuildpack{ID: b.ID, Version: b.Version})
	}

	data := struct {
		Repository string             `toml:"repository"`
		Buildpacks []*SimpleBuildpack `toml:"buildpacks"`
	}{
		Repository: g.Repository,
		Buildpacks: buildpacks,
	}

	return WriteTOML(path, data)
}

func (m BuildpackMap) ReadGroup(path string) (BuildpackGroup, error) {
	var group BuildpackGroup
	if _, err := toml.DecodeFile(path, &group); err != nil {
		return BuildpackGroup{}, err
	}
	group.Buildpacks = m.mapFull(group.Buildpacks)
	return group, nil
}
