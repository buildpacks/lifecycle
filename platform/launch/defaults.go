package launch

import "path/filepath"

const (
	EnvAppDir      = "CNB_APP_DIR"
	EnvLayersDir   = "CNB_LAYERS_DIR"
	EnvNoColor     = "CNB_NO_COLOR" // defaults to false
	EnvPlatformAPI = "CNB_PLATFORM_API"
	EnvProcessType = "CNB_PROCESS_TYPE"

	DefaultPlatformAPI = "0.3"
	DefaultProcessType = "web"
)

var (
	DefaultAppDir    = filepath.Join(rootDir, "workspace")
	DefaultLayersDir = filepath.Join(rootDir, "layers")
)
