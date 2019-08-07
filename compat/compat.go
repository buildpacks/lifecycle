package compat

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpack/lifecycle"
)

type orderConfig struct {
	Groups []groupConfig `toml:"groups"`
}

type groupConfig struct {
	Buildpacks []buildpackRefConfig `toml:"buildpacks"`
}

type buildpackRefConfig struct {
	ID       string `toml:"id"`
	Version  string `toml:"version"`
	Optional bool   `toml:"optional,omitempty"`
}

func (b buildpackRefConfig) dir() string {
	return strings.Replace(b.ID, "/", "_", -1)
}

func ReadOrder(path, buildpacksDir string) (lifecycle.BuildpackOrder, error) {
	var legacyOrder orderConfig
	if _, err := toml.DecodeFile(path, &legacyOrder); err != nil {
		return nil, errors.Wrap(err, "decoding legacy order config")
	}

	return fromLegacy(legacyOrder, buildpacksDir)
}

func fromLegacy(legacyOrder orderConfig, buildpacksDir string) (lifecycle.BuildpackOrder, error) {
	var order lifecycle.BuildpackOrder
	for _, legacyGroup := range legacyOrder.Groups {
		var bps []lifecycle.Buildpack
		for _, legacyBuildpack := range legacyGroup.Buildpacks {
			version, err := resolveVersion(legacyBuildpack, buildpacksDir)
			if err != nil {
				return nil, err
			}
			bps = append(bps, lifecycle.Buildpack{
				ID:       legacyBuildpack.ID,
				Version:  version,
				Optional: legacyBuildpack.Optional,
			})
		}
		order = append(order, lifecycle.BuildpackGroup{
			Group: bps,
		})
	}
	return order, nil
}

type buildpackTOML struct {
	Buildpack buildpack `toml:"buildpack"`
}

type buildpack struct {
	ID      string `toml:"id"`
	Version string `toml:"version"`
}

func resolveVersion(bpRef buildpackRefConfig, buildpacksDir string) (string, error) {
	if bpRef.Version != "latest" {
		return bpRef.Version, nil
	}

	bpDir, err := filepath.Abs(filepath.Join(buildpacksDir, bpRef.dir()))
	if err != nil {
		return "", err
	}

	tomlPaths, err := filepath.Glob(path.Join(bpDir, "*", "buildpack.toml"))
	if err != nil {
		return "", err
	}

	var matchVersions []string
	for _, tomlPath := range tomlPaths {
		bpTOML := buildpackTOML{}
		if _, err := toml.DecodeFile(tomlPath, &bpTOML); err != nil {
			return "", err
		}

		if bpTOML.Buildpack.ID == bpRef.ID {
			matchVersions = append(matchVersions, bpTOML.Buildpack.Version)
		}
	}

	if len(matchVersions) == 0 {
		return "", errors.Errorf("no buildpacks with matching ID '%s'", bpRef.ID)
	}

	if len(matchVersions) > 1 {
		return "", errors.Errorf("too many buildpacks with matching ID '%s'", bpRef.ID)
	}

	return matchVersions[0], nil
}
