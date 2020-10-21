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
	DefaultAnalyzedFileName        = "analyzed.toml"
	DefaultAppDir                  = filepath.Join(rootDir, "workspace")
	DefaultBuildpacksDir           = filepath.Join(rootDir, "cnb", "buildpacks")
	DefaultDeprecationMode         = DeprecationModeWarn
	DefaultGroupFileName           = "group.toml"
	DefaultLauncherPath            = filepath.Join(rootDir, "cnb", "lifecycle", "launcher"+execExt)
	DefaultLayersDir               = filepath.Join(rootDir, "layers")
	DefaultLogLevel                = "info"
	DefaultOrderPath               = filepath.Join(rootDir, "cnb", "order.toml")
	DefaultPlanFileName            = "plan.toml"
	DefaultPlatformAPI             = "0.3"
	DefaultPlatformDir             = filepath.Join(rootDir, "platform")
	DefaultProcessType             = "web"
	DefaultProjectMetadataFileName = "project-metadata.toml"
	DefaultReportFileName          = "report.toml"
	DefaultStackPath               = filepath.Join(rootDir, "cnb", "stack.toml")
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

func FlagAnalyzedPath(version, layersDir string, dir *string) {
	analyzedDir := getRelativePath(version, layersDir)
	flagSet.StringVar(dir, "analyzed", EnvOrDefault(EnvAnalyzedPath, filepath.Join(analyzedDir, DefaultAnalyzedFileName)), "path to analyzed.toml")
}

func FlagAppDir(dir *string) {
	flagSet.StringVar(dir, "app", EnvOrDefault(EnvAppDir, DefaultAppDir), "path to app directory")
}

func FlagBuildpacksDir(dir *string) {
	flagSet.StringVar(dir, "buildpacks", EnvOrDefault(EnvBuildpacksDir, DefaultBuildpacksDir), "path to buildpacks directory")
}

func FlagCacheDir(dir *string) {
	flagSet.StringVar(dir, "cache-dir", os.Getenv(EnvCacheDir), "path to cache directory")
}

func FlagCacheImage(image *string) {
	flagSet.StringVar(image, "cache-image", os.Getenv(EnvCacheImage), "cache image tag name")
}

func FlagGID(gid *int) {
	flagSet.IntVar(gid, "gid", intEnv(EnvGID), "GID of user's group in the stack's build and run images")
}

func FlagGroupPath(version, layersDir string, path *string) {
	groupDir := getRelativePath(version, layersDir)
	flagSet.StringVar(path, "group", EnvOrDefault(EnvGroupPath, filepath.Join(groupDir, DefaultGroupFileName)), "path to group.toml")
}

func FlagLaunchCacheDir(dir *string) {
	flagSet.StringVar(dir, "launch-cache", os.Getenv(EnvLaunchCacheDir), "path to launch cache directory")
}

func FlagLauncherPath(path *string) {
	flagSet.StringVar(path, "launcher", DefaultLauncherPath, "path to launcher binary")
}

func FlagLayersDir(dir *string) {
	flagSet.StringVar(dir, "layers", EnvOrDefault(EnvLayersDir, DefaultLayersDir), "path to layers directory")
}

func FlagNoColor(skip *bool) {
	flagSet.BoolVar(skip, "no-color", BoolEnv(EnvNoColor), "disable color output")
}

func FlagOrderPath(path *string) {
	flagSet.StringVar(path, "order", EnvOrDefault(EnvOrderPath, DefaultOrderPath), "path to order.toml")
}

func FlagPlanPath(version, layersDir string, path *string) {
	planDir := getRelativePath(version, layersDir)
	flagSet.StringVar(path, "plan", EnvOrDefault(EnvPlanPath, filepath.Join(planDir, DefaultPlanFileName)), "path to plan.toml")
}

func FlagPlatformDir(dir *string) {
	flagSet.StringVar(dir, "platform", EnvOrDefault(EnvPlatformDir, DefaultPlatformDir), "path to platform directory")
}

func FlagPreviousImage(image *string) {
	flagSet.StringVar(image, "previous-image", os.Getenv(EnvPreviousImage), "reference to previous image")
}

func FlagReportPath(version, layersDir string, path *string) {
	reportDir := getRelativePath(version, layersDir)
	flagSet.StringVar(path, "report", EnvOrDefault(EnvReportPath, filepath.Join(reportDir, DefaultReportFileName)), "path to report.toml")
}

func FlagRunImage(image *string) {
	flagSet.StringVar(image, "run-image", os.Getenv(EnvRunImage), "reference to run image")
}

func FlagSkipLayers(skip *bool) {
	flagSet.BoolVar(skip, "skip-layers", BoolEnv(EnvSkipLayers), "do not provide layer metadata to buildpacks")
}

func FlagSkipRestore(skip *bool) {
	flagSet.BoolVar(skip, "skip-restore", BoolEnv(EnvSkipRestore), "do not restore layers or layer metadata")
}

func FlagStackPath(path *string) {
	flagSet.StringVar(path, "stack", EnvOrDefault(EnvStackPath, DefaultStackPath), "path to stack.toml")
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

func FlagProjectMetadataPath(version, layersDir string, projectMetadataPath *string) {
	projectMetadataDir := getRelativePath(version, layersDir)
	flagSet.StringVar(projectMetadataPath, "project-metadata", EnvOrDefault(EnvProjectMetadataPath, filepath.Join(projectMetadataDir, DefaultProjectMetadataFileName)), "path to project-metadata.toml")
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

func isVersionLessThan05(version string) bool {
	return api.MustParse(version).Compare(api.MustParse("0.5")) < 0
}

func getRelativePath(version, layersDir string) string {
	if layersDir == "" || isVersionLessThan05(version) {
		return "."
	}
	return layersDir
}
