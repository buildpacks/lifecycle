package cli

import (
	"os"
	"strconv"

	platform "github.com/buildpacks/lifecycle/platform/launch"
)

const (
	EnvNoColor = "CNB_NO_COLOR" // defaults to false
)

var (
	DefaultPlatformAPI = "0.3"

	appDir      = envOrDefault(platform.EnvAppDir, platform.DefaultAppDir)
	layersDir   = envOrDefault(platform.EnvLayersDir, platform.DefaultLayersDir)
	noColor     = boolEnv(EnvNoColor)
	platformAPI = envOrDefault(platform.EnvPlatformAPI, DefaultPlatformAPI)
	processType = envOrDefault(platform.EnvProcessType, platform.DefaultProcessType)
)

func boolEnv(k string) bool {
	v := os.Getenv(k)
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

func envOrDefault(key string, defaultVal string) string {
	if envVal := os.Getenv(key); envVal != "" {
		return envVal
	}
	return defaultVal
}
