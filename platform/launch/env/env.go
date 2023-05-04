package env

import (
	"os"
	"strconv"
)

const (
	VarAppDir          = "CNB_APP_DIR"
	VarDeprecationMode = "CNB_DEPRECATION_MODE"
	VarLayersDir       = "CNB_LAYERS_DIR"
	VarNoColor         = "CNB_NO_COLOR" // defaults to false
	VarPlatformAPI     = "CNB_PLATFORM_API"
	VarProcessType     = "CNB_PROCESS_TYPE"
)

var (
	AppDir          = func() string { return os.Getenv(VarAppDir) }()
	DeprecationMode = func() string { return os.Getenv(VarDeprecationMode) }()
	LayersDir       = func() string { return os.Getenv(VarLayersDir) }()
	NoColor         = func() bool { return boolEnv(VarNoColor) }()
	PlatformAPI     = func() string { return os.Getenv(VarPlatformAPI) }()
	ProcessType     = func() string { return os.Getenv(VarProcessType) }()
)

func boolEnv(k string) bool {
	v := os.Getenv(k)
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}
