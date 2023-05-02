package launch

import (
	"path/filepath"

	"github.com/buildpacks/lifecycle/internal/path"
)

const (
	DefaultPlatformAPI = "0.3"
	DefaultProcessType = "web"
)

var (
	DefaultAppDir    = filepath.Join(path.RootDir, "workspace")
	DefaultLayersDir = filepath.Join(path.RootDir, "layers")
)
