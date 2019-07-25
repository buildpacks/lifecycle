package lifecycle

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
	ID      string         `toml:"id"`
	Version string         `toml:"version"`
	Name    string         `toml:"name"`
	Order   BuildpackOrder `toml:"order"`
	Path    string         `toml:"-"`
}

func (bp buildpackTOML) String() string {
	return bp.Name + " " + bp.Version
}
