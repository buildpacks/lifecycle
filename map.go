package lifecycle

import (
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

const buildpackVersionLatest = "latest"

type BuildpackMap struct {
	PathByID string
}

type buildpackTOML struct {
	Buildpacks []buildpackInfo `toml:"buildpacks"`
}

type buildpackInfo struct {
	ID      string         `toml:"id"`
	Version string         `toml:"version"`
	Name    string         `toml:"name"`
	Path    string         `toml:"path"`
	Order   BuildpackOrder `toml:"order"`
}

//func NewBuildpackMap(path string) (BuildpackMap, error) {
//	buildpacks := BuildpackMap{}
//	glob := filepath.Join(blobDir, "*", "buildpack.toml")
//	files, err := filepath.Glob(glob)
//	if err != nil {
//		return nil, err
//	}
//	for _, file := range files {
//		buildpackDir := filepath.Dir(file)
//		var bpTOML buildpackTOML
//		if _, err := toml.DecodeFile(file, &bpTOML); err != nil {
//			return nil, err
//		}
//
//		_, version := filepath.Split(buildpackDir)
//		key := bpTOML.Buildpack.ID + "@" + version
//		if version != buildpackVersionLatest {
//			key = bpTOML.Buildpack.ID + "@" + bpTOML.Buildpack.Version
//		}
//
//		buildpacks[key] = &Buildpack{
//			ID:      bpTOML.Buildpack.ID,
//			Version: bpTOML.Buildpack.Version,
//			Name:    bpTOML.Buildpack.Name,
//			Path:    buildpackDir,
//		}
//	}
//	return buildpacks, nil
//}

//func (m BuildpackMap) lookupOrder(order BuildpackOrder) (BuildpackOrder, error) {
//	var groups BuildpackOrder
//	for _, g := range order {
//		group, err := m.lookupGroup(g)
//		if err != nil {
//			return nil, errors.Wrap(err, "lookup buildpacks")
//		}
//		groups = append(groups, group)
//	}
//	return groups, nil
//}
//
//func (m BuildpackMap) lookupGroup(g BuildpackGroup) (BuildpackGroup, error) {
//	out := make([]Buildpack, 0, len(g.Group))
//	for _, b := range g.Group {
//		bpTOML := buildpackTOML{
//			path: filepath.Join(m.PathByID, b.ID, b.Version),
//			seen: map[string]struct{}{},
//		}
//		if _, err := toml.DecodeFile(filepath.Join(bpTOML.path, "buildpack.toml"), &bpTOML); err != nil {
//			return BuildpackGroup{}, err
//		}
//		b, err := bpTOML.lookup(b)
//
//	}
//	return BuildpackGroup{Group: out}, nil
//}
//
//func (bt *buildpackTOML) lookup(buildpack Buildpack) (Buildpack, error) {
//	for _, b := range bt.Buildpacks {
//		if b.ID == buildpack.ID && b.Version == buildpack.Version {
//
//			if b.Order != nil && b.Path != "" {
//				return Buildpack{}, errors.Errorf("invalid buildpack '%s@%s'", b.ID, b.Version)
//			}
//			buildpack.Name = b.Name
//			buildpack.Path = b.Path
//
//			if b.Order != nil {
//
//			}
//			return buildpack, nil
//		}
//	}
//}

func (m BuildpackMap) ReadOrder(orderPath string) (BuildpackOrder, error) {
	var order struct {
		Order BuildpackOrder `toml:"order"`
	}
	if _, err := toml.DecodeFile(orderPath, &order); err != nil {
		return nil, err
	}
	return m.lookupOrder(order.Order)
}

func (g *BuildpackGroup) Write(path string) error {
	data := struct {
		Buildpacks []Buildpack `toml:"buildpacks"`
	}{
		Buildpacks: g.Group,
	}
	return WriteTOML(path, data)
}

func (m BuildpackMap) ReadGroup(path string) (*BuildpackGroup, error) {
	var group BuildpackGroup
	var err error
	if _, err := toml.DecodeFile(path, &group); err != nil {
		return nil, err
	}
	group, err = m.lookupGroup(group)
	if err != nil {
		return nil, errors.Wrap(err, "lookup buildpacks")
	}
	return &group, nil
}

func ReadOrder(path string) (BuildpackOrder, error) {
	var order struct {
		Order BuildpackOrder `toml:"order"`
	}
	if _, err := toml.DecodeFile(path, &order); err != nil {
		return nil, err
	}
	return order.Order, nil
}
