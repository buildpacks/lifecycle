package cli

import (
	"os"
	"strconv"

	"github.com/buildpacks/lifecycle/platform"
)

const (
	EnvNoColor = "CNB_NO_COLOR" // defaults to false
)

var (
	PlatformAPI = envOrDefault(platform.EnvPlatformAPI, DefaultPlatformAPI)

	DefaultLogLevel    = "info"
	DefaultPlatformAPI = "0.3"

	analyzedPath        = envOrDefault(platform.EnvAnalyzedPath, platform.PlaceholderAnalyzedPath)
	appDir              = envOrDefault(platform.EnvAppDir, platform.DefaultAppDir)
	cacheDir            = os.Getenv(platform.EnvCacheDir)
	cacheImage          = os.Getenv(platform.EnvCacheImage)
	buildpacksDir       = envOrDefault(platform.EnvBuildpacksDir, platform.DefaultBuildpacksDir)
	groupPath           = envOrDefault(platform.EnvGroupPath, platform.PlaceholderGroupPath)
	gid                 = intEnv(platform.EnvGID)
	launchCacheDir      = os.Getenv(platform.EnvLaunchCacheDir)
	layersDir           = envOrDefault(platform.EnvLayersDir, platform.DefaultLayersDir)
	logLevel            = envOrDefault(platform.EnvLogLevel, DefaultLogLevel)
	noColor             = boolEnv(EnvNoColor)
	orderPath           = envOrDefault(platform.EnvOrderPath, platform.PlaceholderOrderPath)
	planPath            = envOrDefault(platform.EnvPlanPath, platform.PlaceholderPlanPath)
	platformDir         = envOrDefault(platform.EnvPlatformDir, platform.DefaultPlatformDir)
	previousImage       = os.Getenv(platform.EnvPreviousImage)
	processType         = os.Getenv(platform.EnvProcessType)
	projectMetadataPath = envOrDefault(platform.EnvProjectMetadataPath, platform.PlaceholderProjectMetadataPath)
	reportPath          = envOrDefault(platform.EnvReportPath, platform.PlaceholderReportPath)
	runImage            = os.Getenv(platform.EnvRunImage)
	skipLayers          = boolEnv(platform.EnvSkipLayers)
	skipRestore         = boolEnv(platform.EnvSkipRestore)
	stackPath           = envOrDefault(platform.EnvStackPath, platform.DefaultStackPath)
	uid                 = intEnv(platform.EnvUID)
	useDaemon           = boolEnv(platform.EnvUseDaemon)
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

func intEnv(k string) int {
	v := os.Getenv(k)
	d, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return d
}
