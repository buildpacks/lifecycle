package cli

import (
	"flag"
	"os"
	"path/filepath"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/str"
	"github.com/buildpacks/lifecycle/platform"
)

var flagSet = flag.NewFlagSet("lifecycle", flag.ExitOnError)

func FlagAnalyzedPath(provided *string) {
	flagSet.StringVar(provided, "analyzed", analyzedPath, "path to analyzed.toml")
}

func DefaultAnalyzedPath(platformAPI, layersDir string) string {
	return defaultPath(platform.DefaultAnalyzedFile, platformAPI, layersDir)
}

func FlagAppDir(provided *string) {
	flagSet.StringVar(provided, "app", appDir, "path to app directory")
}

func FlagBuildpacksDir(provided *string) {
	flagSet.StringVar(provided, "buildpacks", buildpacksDir, "path to buildpacks directory")
}

func FlagCacheDir(provided *string) {
	flagSet.StringVar(provided, "cache-dir", cacheDir, "path to cache directory")
}

func FlagCacheImage(provided *string) {
	flagSet.StringVar(provided, "cache-image", cacheImage, "cache image tag name")
}

func FlagGID(provided *int) {
	flagSet.IntVar(provided, "gid", gid, "GID of user's group in the stack's build and run images")
}

func FlagGroupPath(provided *string) {
	flagSet.StringVar(provided, "group", groupPath, "path to group.toml")
}

func DefaultGroupPath(platformAPI, layersDir string) string {
	return defaultPath(platform.DefaultGroupFile, platformAPI, layersDir)
}

func FlagLaunchCacheDir(provided *string) {
	flagSet.StringVar(provided, "launch-cache", launchCacheDir, "path to launch cache directory")
}

func FlagLauncherPath(launcherPath *string) {
	flagSet.StringVar(launcherPath, "launcher", platform.DefaultLauncherPath, "path to launcher binary")
}

func FlagLayersDir(provided *string) {
	flagSet.StringVar(provided, "layers", layersDir, "path to layers directory")
}

func FlagLogLevel(provided *string) {
	flagSet.StringVar(provided, "log-level", logLevel, "logging level")
}

func FlagNoColor(provided *bool) {
	flagSet.BoolVar(provided, "no-color", noColor, "disable color output")
}

func FlagOrderPath(provided *string) {
	flagSet.StringVar(provided, "order", orderPath, "path to order.toml")
}

// TODO: remove when https://github.com/buildpacks/lifecycle/pull/860 is merged
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

func FlagPlanPath(provided *string) {
	flagSet.StringVar(provided, "plan", planPath, "path to plan.toml")
}

func DefaultPlanPath(platformAPI, layersDir string) string {
	return defaultPath(platform.DefaultPlanFile, platformAPI, layersDir)
}

func FlagPlatformDir(provided *string) {
	flagSet.StringVar(provided, "platform", platformDir, "path to platform directory")
}

func FlagPreviousImage(provided *string) {
	flagSet.StringVar(provided, "previous-image", previousImage, "reference to previous image")
}

func FlagProcessType(provided *string) {
	flagSet.StringVar(provided, "process-type", processType, "default process type")
}

func FlagProjectMetadataPath(provided *string) {
	flagSet.StringVar(provided, "project-metadata", projectMetadataPath, "path to project-metadata.toml")
}

func DefaultProjectMetadataPath(platformAPI, layersDir string) string {
	return defaultPath(platform.DefaultProjectMetadataFile, platformAPI, layersDir)
}

func FlagReportPath(provided *string) {
	flagSet.StringVar(provided, "report", reportPath, "path to report.toml")
}

func DefaultReportPath(platformAPI, layersDir string) string {
	return defaultPath(platform.DefaultReportFile, platformAPI, layersDir)
}

func FlagRunImage(provided *string) {
	flagSet.StringVar(provided, "run-image", runImage, "reference to run image")
}

func FlagSkipLayers(provided *bool) {
	flagSet.BoolVar(provided, "skip-layers", skipLayers, "do not provide layer metadata to buildpacks")
}

func FlagSkipRestore(provided *bool) {
	flagSet.BoolVar(provided, "skip-restore", skipRestore, "do not restore layers or layer metadata")
}

func FlagStackPath(provided *string) {
	flagSet.StringVar(provided, "stack", stackPath, "path to stack.toml")
}

func FlagTags(tags *str.Slice) {
	flagSet.Var(tags, "tag", "additional tags")
}

func FlagUID(provided *int) {
	flagSet.IntVar(provided, "uid", uid, "UID of user in the stack's build and run images")
}

func FlagUseDaemon(provided *bool) {
	flagSet.BoolVar(provided, "daemon", useDaemon, "export to docker daemon")
}

func FlagVersion(provided *bool) {
	flagSet.BoolVar(provided, "version", false, "show version")
}

func DeprecatedFlagRunImage(provided *string) {
	flagSet.StringVar(provided, "image", "", "reference to run image")
}

// TODO: remove when https://github.com/buildpacks/lifecycle/pull/860 is merged
func defaultPath(fileName, platformAPI, layersDir string) string {
	if (api.MustParse(platformAPI).LessThan("0.5")) || (layersDir == "") {
		// prior to platform api 0.5, the default directory was the working dir.
		// layersDir is unset when this call comes from the rebaser - will be fixed as part of https://github.com/buildpacks/spec/issues/156
		return filepath.Join(".", fileName)
	}
	return filepath.Join(layersDir, fileName) // starting from platform api 0.5, the default directory is the layers dir.
}
