package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/buildpacks/lifecycle/api"
)

var (
	DefaultAnalyzedPath        = "<layers>/analyzed.toml"
	DefaultAppDir              = filepath.Join(rootDir, "workspace")
	DefaultBuildpacksDir       = filepath.Join(rootDir, "cnb", "buildpacks")
	DefaultDeprecationMode     = DeprecationModeWarn
	DefaultGroupPath           = "<layers>/group.toml"
	DefaultLauncherPath        = filepath.Join(rootDir, "cnb", "lifecycle", "launcher"+execExt)
	DefaultLayersDir           = filepath.Join(rootDir, "layers")
	DefaultLogLevel            = "info"
	DefaultOrderPath           = filepath.Join(rootDir, "cnb", "order.toml")
	DefaultPlanPath            = "<layers>/plan.toml"
	DefaultPlatformAPI         = "0.3"
	DefaultPlatformDir         = filepath.Join(rootDir, "platform")
	DefaultProcessType         = "web"
	DefaultProjectMetadataPath = "<layers>/project-metadata.toml"
	DefaultReportPath          = "<layers>/report.toml"
	DefaultStackPath           = filepath.Join(rootDir, "cnb", "stack.toml")
)

const (
	EnvAnalyzedPath        = "CNB_ANALYZED_PATH"
	EnvAppDir              = "CNB_APP_DIR"
	EnvBuildpacksDir       = "CNB_BUILDPACKS_DIR"
	EnvCacheDir            = "CNB_CACHE_DIR"
	EnvCacheImage          = "CNB_CACHE_IMAGE"
	EnvDeprecationMode     = "CNB_DEPRECATION_MODE"
	EnvGID                 = "CNB_GROUP_ID"
	EnvGroupPath           = "CNB_GROUP_PATH"
	EnvLaunchCacheDir      = "CNB_LAUNCH_CACHE_DIR"
	EnvLayersDir           = "CNB_LAYERS_DIR"
	EnvLogLevel            = "CNB_LOG_LEVEL"
	EnvNoColor             = "CNB_NO_COLOR" // defaults to false
	EnvOrderPath           = "CNB_ORDER_PATH"
	EnvPlanPath            = "CNB_PLAN_PATH"
	EnvPlatformAPI         = "CNB_PLATFORM_API"
	EnvPlatformDir         = "CNB_PLATFORM_DIR"
	EnvPreviousImage       = "CNB_PREVIOUS_IMAGE"
	EnvProcessType         = "CNB_PROCESS_TYPE"
	EnvProjectMetadataPath = "CNB_PROJECT_METADATA_PATH"
	EnvRegistryAuth        = "CNB_REGISTRY_AUTH"
	EnvReportPath          = "CNB_REPORT_PATH"
	EnvRunImage            = "CNB_RUN_IMAGE"
	EnvSkipLayers          = "CNB_ANALYZE_SKIP_LAYERS" // defaults to false
	EnvSkipRestore         = "CNB_SKIP_RESTORE"        // defaults to false
	EnvStackPath           = "CNB_STACK_PATH"
	EnvUID                 = "CNB_USER_ID"
	EnvUseDaemon           = "CNB_USE_DAEMON" // defaults to false
)

var flagSet = flag.NewFlagSet("lifecycle", flag.ExitOnError)

func FlagAnalyzedPath(analyzedPath *string) {
	flagSet.StringVar(analyzedPath, "analyzed", EnvOrDefault(EnvAnalyzedPath, DefaultAnalyzedPath), "path to analyzed.toml")
}
func UpdateAnalyzedPath(analyzedPath *string, platformAPI, layersDir string) {
	updatePath(analyzedPath, DefaultAnalyzedPath, platformAPI, layersDir)
}

func FlagAppDir(appDir *string) {
	flagSet.StringVar(appDir, "app", EnvOrDefault(EnvAppDir, DefaultAppDir), "path to app directory")
}

func FlagBuildpacksDir(buildpacksDir *string) {
	flagSet.StringVar(buildpacksDir, "buildpacks", EnvOrDefault(EnvBuildpacksDir, DefaultBuildpacksDir), "path to buildpacks directory")
}

func FlagCacheDir(cacheDir *string) {
	flagSet.StringVar(cacheDir, "cache-dir", os.Getenv(EnvCacheDir), "path to cache directory")
}

func FlagCacheImage(cacheImage *string) {
	flagSet.StringVar(cacheImage, "cache-image", os.Getenv(EnvCacheImage), "cache image tag name")
}

func FlagGID(gid *int) {
	flagSet.IntVar(gid, "gid", intEnv(EnvGID), "GID of user's group in the stack's build and run images")
}

func FlagGroupPath(groupPath *string) {
	flagSet.StringVar(groupPath, "group", EnvOrDefault(EnvGroupPath, DefaultGroupPath), "path to group.toml")
}

func UpdateGroupPath(groupPath *string, platformAPI, layersDir string) {
	updatePath(groupPath, DefaultGroupPath, platformAPI, layersDir)
}

func FlagLaunchCacheDir(launchCacheDir *string) {
	flagSet.StringVar(launchCacheDir, "launch-cache", os.Getenv(EnvLaunchCacheDir), "path to launch cache directory")
}

func FlagLauncherPath(launcherPath *string) {
	flagSet.StringVar(launcherPath, "launcher", DefaultLauncherPath, "path to launcher binary")
}

func FlagLayersDir(layersDir *string) {
	flagSet.StringVar(layersDir, "layers", EnvOrDefault(EnvLayersDir, DefaultLayersDir), "path to layers directory")
}

func FlagNoColor(skip *bool) {
	flagSet.BoolVar(skip, "no-color", BoolEnv(EnvNoColor), "disable color output")
}

func FlagOrderPath(orderPath *string) {
	flagSet.StringVar(orderPath, "order", EnvOrDefault(EnvOrderPath, DefaultOrderPath), "path to order.toml")
}

func FlagPlanPath(planPath *string) {
	flagSet.StringVar(planPath, "plan", EnvOrDefault(EnvPlanPath, DefaultPlanPath), "path to plan.toml")
}

func UpdatePlanPath(planPath *string, platformAPI, layersDir string) {
	updatePath(planPath, DefaultPlanPath, platformAPI, layersDir)
}

func FlagPlatformDir(platformDir *string) {
	flagSet.StringVar(platformDir, "platform", EnvOrDefault(EnvPlatformDir, DefaultPlatformDir), "path to platform directory")
}

func FlagPreviousImage(image *string) {
	flagSet.StringVar(image, "previous-image", os.Getenv(EnvPreviousImage), "reference to previous image")
}

func FlagReportPath(reportPath *string) {
	flagSet.StringVar(reportPath, "report", EnvOrDefault(EnvReportPath, DefaultReportPath), "path to report.toml")
}

func UpdateReportPath(reportPath *string, platformAPI, layersDir string) {
	updatePath(reportPath, DefaultReportPath, platformAPI, layersDir)
}

func FlagRunImage(runImage *string) {
	flagSet.StringVar(runImage, "run-image", os.Getenv(EnvRunImage), "reference to run image")
}

func FlagSkipLayers(skip *bool) {
	flagSet.BoolVar(skip, "skip-layers", BoolEnv(EnvSkipLayers), "do not provide layer metadata to buildpacks")
}

func FlagSkipRestore(skip *bool) {
	flagSet.BoolVar(skip, "skip-restore", BoolEnv(EnvSkipRestore), "do not restore layers or layer metadata")
}

func FlagStackPath(stackPath *string) {
	flagSet.StringVar(stackPath, "stack", EnvOrDefault(EnvStackPath, DefaultStackPath), "path to stack.toml")
}

func FlagTags(tags *StringSlice) {
	flagSet.Var(tags, "tag", "additional tags")
}

func FlagUID(uid *int) {
	flagSet.IntVar(uid, "uid", intEnv(EnvUID), "UID of user in the stack's build and run images")
}

func FlagUseDaemon(use *bool) {
	flagSet.BoolVar(use, "daemon", BoolEnv(EnvUseDaemon), "export to docker daemon")
}

func FlagVersion(version *bool) {
	flagSet.BoolVar(version, "version", false, "show version")
}

func FlagLogLevel(level *string) {
	flagSet.StringVar(level, "log-level", EnvOrDefault(EnvLogLevel, DefaultLogLevel), "logging level")
}

func FlagProjectMetadataPath(projectMetadataPath *string) {
	flagSet.StringVar(projectMetadataPath, "project-metadata", EnvOrDefault(EnvProjectMetadataPath, DefaultProjectMetadataPath), "path to project-metadata.toml")
}

func UpdateProjectMetadataPath(projectMetadataPath *string, platformAPI, layersDir string) {
	updatePath(projectMetadataPath, DefaultProjectMetadataPath, platformAPI, layersDir)
}

func FlagProcessType(processType *string) {
	flagSet.StringVar(processType, "process-type", os.Getenv(EnvProcessType), "default process type")
}

func DeprecatedFlagRunImage(image *string) {
	flagSet.StringVar(image, "image", "", "reference to run image")
}

type StringSlice []string

func (s *StringSlice) String() string {
	return fmt.Sprintf("%+v", *s)
}

func (s *StringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func intEnv(k string) int {
	v := os.Getenv(k)
	d, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return d
}

func BoolEnv(k string) bool {
	v := os.Getenv(k)
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

func EnvOrDefault(key string, defaultVal string) string {
	if envVal := os.Getenv(key); envVal != "" {
		return envVal
	}
	return defaultVal
}

func updatePath(pathToUpdate *string, defaultPath, platformAPI, layersDir string) {
	if *pathToUpdate != defaultPath {
		return
	}
	fileName := filepath.Base(defaultPath)
	if isPlatformAPILessThan05(platformAPI) || layersDir == "" {
		// layersDir is unset when this call comes from the rebaser - will be fixed as part of https://github.com/buildpacks/spec/issues/156
		*pathToUpdate = filepath.Join(".", fileName)
	} else {
		*pathToUpdate = filepath.Join(layersDir, fileName)
	}
}

func isPlatformAPILessThan05(platformAPI string) bool {
	return api.MustParse(platformAPI).Compare(api.MustParse("0.5")) < 0
}
