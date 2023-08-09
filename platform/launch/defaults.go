package launch

import (
	"path/filepath"

	"github.com/buildpacks/lifecycle/internal/path"
)

const (
	EnvAppDir      = "CNB_APP_DIR"
	EnvLayersDir   = "CNB_LAYERS_DIR"
	EnvNoColor     = "CNB_NO_COLOR" // defaults to false
	EnvPlatformAPI = "CNB_PLATFORM_API"
	EnvProcessType = "CNB_PROCESS_TYPE"

	DefaultPlatformAPI = ""
	DefaultProcessType = "web"

	DefaultExecExt = path.ExecExt
)

var (
	DefaultAppDir    = filepath.Join(path.RootDir, "workspace")
	DefaultLayersDir = filepath.Join(path.RootDir, "layers")
)
