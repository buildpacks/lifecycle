package launch

import "path/filepath"

const (
	EnvAppDir      = "CNB_APP_DIR"
	EnvLayersDir   = "CNB_LAYERS_DIR"
	EnvPlatformAPI = "CNB_PLATFORM_API"
	EnvProcessType = "CNB_PROCESS_TYPE"
)

var (
	DefaultAppDir      = filepath.Join(rootDir, "workspace")
	DefaultLayersDir   = filepath.Join(rootDir, "layers")
	DefaultProcessType = "web"
)
