package lifecycle

import "github.com/pkg/errors"

var POSIXBuildEnv = map[string][]string{
	"bin": {
		"PATH",
	},
	"lib": {
		"LD_LIBRARY_PATH",
		"LIBRARY_PATH",
	},
	"include": {
		"CPATH",
		"C_INCLUDE_PATH",
		"CPLUS_INCLUDE_PATH",
		"OBJC_INCLUDE_PATH",
	},
	"pkgconfig": {
		"PKG_CONFIG_PATH",
	},
}

var POSIXLaunchEnv = map[string][]string{
	"bin": {"PATH"},
	"lib": {"LD_LIBRARY_PATH"},
}

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
	TOML    string         `toml:"-"`
}

func (bp buildpackInfo) String() string {
	return bp.Name + " " + bp.Version
}
