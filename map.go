package lifecycle

import (
	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

type buildpackTOML struct {
	Buildpacks []buildpackInfo `toml:"buildpacks"`
}

func (bt *buildpackTOML) lookup(bp Buildpack) (*buildpackInfo, error) {
	for _, b := range bt.Buildpacks {
		if b.ID == bp.ID && b.Version == bp.Version {

			if b.Order != nil && b.Path != "" {
				return nil, errors.Errorf("invalid buildpack '%s'", bp)
			}

			if b.Order == nil && b.Path == "" {
				b.Path = "."
			}

			// TODO: validate that stack matches $BP_STACK_ID
			// TODO: validate that orders don't have stacks

			return &b, nil
		}
	}
	return nil, errors.Errorf("could not find buildpack '%s'", bp)
}

type buildpackInfo struct {
	ID      string         `toml:"id"`
	Version string         `toml:"version"`
	Name    string         `toml:"name"`
	Path    string         `toml:"path"`
	Order   BuildpackOrder `toml:"order"`
}

func (g *BuildpackGroup) Write(path string) error {
	data := struct {
		Buildpacks []Buildpack `toml:"buildpacks"`
	}{
		Buildpacks: g.Group,
	}
	return WriteTOML(path, data)
}

func ReadGroup(path string) (BuildpackGroup, error) {
	var group BuildpackGroup
	_, err := toml.DecodeFile(path, &group)
	return group, err
}

func ReadOrder(path string) (BuildpackOrder, error) {
	var order struct {
		Order BuildpackOrder `toml:"order"`
	}
	_, err := toml.DecodeFile(path, &order)
	return order.Order, err
}
