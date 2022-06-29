package cli

import (
	"flag"
	"os"
	"path/filepath"
	"strconv"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/str"
	"github.com/buildpacks/lifecycle/platform"
)

var flagSet = flag.NewFlagSet("lifecycle", flag.ExitOnError)

func FlagAnalyzedPath(analyzedPath *string) {
	flagSet.StringVar(analyzedPath, "analyzed", envOrDefault(platform.EnvAnalyzedPath, platform.PlaceholderAnalyzedPath), "path to analyzed.toml")
}

func DefaultAnalyzedPath(platformAPI, layersDir string) string {
	return defaultPath(platform.DefaultAnalyzedFile, platformAPI, layersDir)
}

func FlagAppDir(appDir *string) {
	flagSet.StringVar(appDir, "app", envOrDefault(platform.EnvAppDir, platform.DefaultAppDir), "path to app directory")
}

func FlagBuildpacksDir(buildpacksDir *string) {
	flagSet.StringVar(buildpacksDir, "buildpacks", envOrDefault(platform.EnvBuildpacksDir, platform.DefaultBuildpacksDir), "path to buildpacks directory")
}

func FlagCacheDir(cacheDir *string) {
	flagSet.StringVar(cacheDir, "cache-dir", os.Getenv(platform.EnvCacheDir), "path to cache directory")
}

func FlagCacheImage(cacheImage *string) {
	flagSet.StringVar(cacheImage, "cache-image", os.Getenv(platform.EnvCacheImage), "cache image tag name")
}

func FlagExtensionsDir(extensionsDir *string) {
	flagSet.StringVar(extensionsDir, "extensions", envOrDefault(platform.EnvExtensionsDir, platform.DefaultExtensionsDir), "path to extensions directory")
}

func FlagGID(gid *int) {
	flagSet.IntVar(gid, "gid", intEnv(platform.EnvGID), "GID of user's group in the stack's build and run images")
}

func FlagGroupPath(groupPath *string) {
	flagSet.StringVar(groupPath, "group", envOrDefault(platform.EnvGroupPath, platform.PlaceholderGroupPath), "path to group.toml")
}

func DefaultGroupPath(platformAPI, layersDir string) string {
	return defaultPath(platform.DefaultGroupFile, platformAPI, layersDir)
}

func FlagLaunchCacheDir(launchCacheDir *string) {
	flagSet.StringVar(launchCacheDir, "launch-cache", os.Getenv(platform.EnvLaunchCacheDir), "path to launch cache directory")
}

func FlagLauncherPath(launcherPath *string) {
	flagSet.StringVar(launcherPath, "launcher", platform.DefaultLauncherPath, "path to launcher binary")
}

func FlagLayersDir(layersDir *string) {
	flagSet.StringVar(layersDir, "layers", envOrDefault(platform.EnvLayersDir, platform.DefaultLayersDir), "path to layers directory")
}

func FlagNoColor(skip *bool) {
	flagSet.BoolVar(skip, "no-color", BoolEnv(platform.EnvNoColor), "disable color output")
}

func FlagOrderPath(orderPath *string) {
	flagSet.StringVar(orderPath, "order", envOrDefault(platform.EnvOrderPath, platform.PlaceholderOrderPath), "path to order.toml")
}

func FlagOutputDir(dir *string) {
	flagSet.StringVar(dir, "output-dir", envOrDefault(platform.EnvOutputDir, platform.DefaultOutputDir), "path to output directory")
}

func DefaultOrderPath(platformAPI, layersDir string) string {
	cnbOrderPath := filepath.Join(rootDir, "cnb", "order.toml")

	// prior to Platform API 0.6, the default is /cnb/order.toml
	if api.MustParse(platformAPI).LessThan("0.6") {
		return cnbOrderPath
	}

	// the default is /<layers>/order.toml or /cnb/order.toml if not present
	layersOrderPath := filepath.Join(layersDir, "order.toml")
	if _, err := os.Stat(layersOrderPath); os.IsNotExist(err) {
		return cnbOrderPath
	}
	return layersOrderPath
}

func FlagPlanPath(planPath *string) {
	flagSet.StringVar(planPath, "plan", envOrDefault(platform.EnvPlanPath, platform.PlaceholderPlanPath), "path to plan.toml")
}

func DefaultPlanPath(platformAPI, layersDir string) string {
	return defaultPath(platform.DefaultPlanFile, platformAPI, layersDir)
}

func FlagPlatformDir(platformDir *string) {
	flagSet.StringVar(platformDir, "platform", envOrDefault(platform.EnvPlatformDir, platform.DefaultPlatformDir), "path to platform directory")
}

func FlagPreviousImage(image *string) {
	flagSet.StringVar(image, "previous-image", os.Getenv(platform.EnvPreviousImage), "reference to previous image")
}

func FlagReportPath(reportPath *string) {
	flagSet.StringVar(reportPath, "report", envOrDefault(platform.EnvReportPath, platform.PlaceholderReportPath), "path to report.toml")
}

func DefaultReportPath(platformAPI, layersDir string) string {
	return defaultPath(platform.DefaultReportFile, platformAPI, layersDir)
}

func FlagRunImage(runImage *string) {
	flagSet.StringVar(runImage, "run-image", os.Getenv(platform.EnvRunImage), "reference to run image")
}

func FlagSkipLayers(skip *bool) {
	flagSet.BoolVar(skip, "skip-layers", BoolEnv(platform.EnvSkipLayers), "do not provide layer metadata to buildpacks")
}

func FlagSkipRestore(skip *bool) {
	flagSet.BoolVar(skip, "skip-restore", BoolEnv(platform.EnvSkipRestore), "do not restore layers or layer metadata")
}

func FlagStackPath(stackPath *string) {
	flagSet.StringVar(stackPath, "stack", envOrDefault(platform.EnvStackPath, platform.DefaultStackPath), "path to stack.toml")
}

func FlagTags(tags *str.Slice) {
	flagSet.Var(tags, "tag", "additional tags")
}

func FlagUID(uid *int) {
	flagSet.IntVar(uid, "uid", intEnv(platform.EnvUID), "UID of user in the stack's build and run images")
}

func FlagUseDaemon(use *bool) {
	flagSet.BoolVar(use, "daemon", BoolEnv(platform.EnvUseDaemon), "export to docker daemon")
}

func FlagVersion(version *bool) {
	flagSet.BoolVar(version, "version", false, "show version")
}

func FlagLogLevel(level *string) {
	flagSet.StringVar(level, "log-level", envOrDefault(platform.EnvLogLevel, platform.DefaultLogLevel), "logging level")
}

func FlagProjectMetadataPath(projectMetadataPath *string) {
	flagSet.StringVar(projectMetadataPath, "project-metadata", envOrDefault(platform.EnvProjectMetadataPath, platform.PlaceholderProjectMetadataPath), "path to project-metadata.toml")
}

func DefaultProjectMetadataPath(platformAPI, layersDir string) string {
	return defaultPath(platform.DefaultProjectMetadataFile, platformAPI, layersDir)
}

func FlagProcessType(processType *string) {
	flagSet.StringVar(processType, "process-type", os.Getenv(platform.EnvProcessType), "default process type")
}

func DeprecatedFlagRunImage(image *string) {
	flagSet.StringVar(image, "image", "", "reference to run image")
}

type StringSlice interface {
	String() string
	Set(value string) error
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

func envOrDefault(key string, defaultVal string) string {
	if envVal := os.Getenv(key); envVal != "" {
		return envVal
	}
	return defaultVal
}

func defaultPath(fileName, platformAPI, layersDir string) string {
	if (api.MustParse(platformAPI).LessThan("0.5")) || (layersDir == "") {
		// prior to platform api 0.5, the default directory was the working dir.
		// layersDir is unset when this call comes from the rebaser - will be fixed as part of https://github.com/buildpacks/spec/issues/156
		return filepath.Join(".", fileName)
	}
	return filepath.Join(layersDir, fileName) // starting from platform api 0.5, the default directory is the layers dir.
}
