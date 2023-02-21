package cli

import (
	"flag"
	"os"
	"strconv"
	"time"

	"github.com/buildpacks/lifecycle/internal/str"
	"github.com/buildpacks/lifecycle/platform"
)

var flagSet = flag.NewFlagSet("lifecycle", flag.ExitOnError)

func FlagAnalyzedPath(analyzedPath *string) {
	flagSet.StringVar(analyzedPath, "analyzed", *analyzedPath, "path to analyzed.toml")
}

func FlagAppDir(appDir *string) {
	flagSet.StringVar(appDir, "app", *appDir, "path to app directory")
}

func FlagBuildConfigDir(buildConfigDir *string) {
	flagSet.StringVar(buildConfigDir, "build-config", *buildConfigDir, "path to build config directory")
}

func FlagBuildImage(buildImage *string) {
	flagSet.StringVar(buildImage, "build-image", *buildImage, "build image tag name")
}

func FlagBuildpacksDir(buildpacksDir *string) {
	flagSet.StringVar(buildpacksDir, "buildpacks", *buildpacksDir, "path to buildpacks directory")
}

func FlagCacheDir(cacheDir *string) {
	flagSet.StringVar(cacheDir, "cache-dir", *cacheDir, "path to cache directory")
}

func FlagCacheImage(cacheImage *string) {
	flagSet.StringVar(cacheImage, "cache-image", *cacheImage, "cache image tag name")
}

func FlagExtendKind(extendKind *string) {
	flagSet.StringVar(extendKind, "kind", *extendKind, "kind of image to extend")
}

func FlagExtendedDir(extendedDir *string) {
	flagSet.StringVar(extendedDir, "extended", *extendedDir, "path to output directory for image layers created from applying generated Dockerfiles")
}

func FlagExtensionsDir(extensionsDir *string) {
	flagSet.StringVar(extensionsDir, "extensions", *extensionsDir, "path to extensions directory")
}

func FlagGID(gid *int) {
	flagSet.IntVar(gid, "gid", *gid, "GID of user's group in the stack's build and run images")
}

func FlagGeneratedDir(generatedDir *string) {
	flagSet.StringVar(generatedDir, "generated", *generatedDir, "path to output directory for files generated by image extensions")
}

func FlagGroupPath(groupPath *string) {
	flagSet.StringVar(groupPath, "group", *groupPath, "path to group.toml")
}

func FlagKanikoCacheTTL(kanikoCacheTTL *time.Duration) {
	flagSet.DurationVar(kanikoCacheTTL, "kaniko-cache-ttl", *kanikoCacheTTL, "kaniko cache time-to-live")
}

func FlagLaunchCacheDir(launchCacheDir *string) {
	flagSet.StringVar(launchCacheDir, "launch-cache", *launchCacheDir, "path to launch cache directory")
}

func FlagLauncherPath(launcherPath *string) {
	flagSet.StringVar(launcherPath, "launcher", *launcherPath, "path to launcher binary")
}

func FlagLauncherSBOMDir(launcherSBOMDir *string) {
	flagSet.StringVar(launcherSBOMDir, "launcher-sbom", *launcherSBOMDir, "path to launcher SBOM directory")
}

func FlagLayersDir(layersDir *string) {
	flagSet.StringVar(layersDir, "layers", *layersDir, "path to layers directory")
}

func FlagLogLevel(logLevel *string) {
	flagSet.StringVar(logLevel, "log-level", platform.DefaultLogLevel, "logging level")
}

func FlagNoColor(noColor *bool) {
	flagSet.BoolVar(noColor, "no-color", boolEnv(platform.EnvNoColor), "disable color output")
}

func FlagOrderPath(orderPath *string) {
	flagSet.StringVar(orderPath, "order", *orderPath, "path to order.toml")
}

func FlagPlanPath(planPath *string) {
	flagSet.StringVar(planPath, "plan", *planPath, "path to plan.toml")
}

func FlagPlatformDir(platformDir *string) {
	flagSet.StringVar(platformDir, "platform", *platformDir, "path to platform directory")
}

func FlagPreviousImage(previousImage *string) {
	flagSet.StringVar(previousImage, "previous-image", *previousImage, "reference to previous image")
}

func FlagProcessType(processType *string) {
	flagSet.StringVar(processType, "process-type", *processType, "default process type")
}

func FlagProjectMetadataPath(projectMetadataPath *string) {
	flagSet.StringVar(projectMetadataPath, "project-metadata", *projectMetadataPath, "path to project-metadata.toml")
}

func FlagReportPath(reportPath *string) {
	flagSet.StringVar(reportPath, "report", *reportPath, "path to report.toml")
}

func FlagRunImage(runImage *string) {
	flagSet.StringVar(runImage, "run-image", *runImage, "reference to run image")
}

func FlagRunPath(runPath *string) {
	flagSet.StringVar(runPath, "run", *runPath, "path to run.toml")
}

func FlagSkipLayers(skipLayers *bool) {
	flagSet.BoolVar(skipLayers, "skip-layers", *skipLayers, "do not provide layer metadata to buildpacks")
}

func FlagSkipRestore(skipRestore *bool) {
	flagSet.BoolVar(skipRestore, "skip-restore", *skipRestore, "do not restore layers or layer metadata")
}

func FlagStackPath(stackPath *string) {
	flagSet.StringVar(stackPath, "stack", *stackPath, "path to stack.toml")
}

func FlagTags(tags *str.Slice) {
	flagSet.Var(tags, "tag", "additional tags")
}

func FlagUID(uid *int) {
	flagSet.IntVar(uid, "uid", *uid, "UID of user in the stack's build and run images")
}

func FlagUseDaemon(useDaemon *bool) {
	flagSet.BoolVar(useDaemon, "daemon", *useDaemon, "export to docker daemon")
}

func FlagVersion(showVersion *bool) {
	flagSet.BoolVar(showVersion, "version", false, "show version")
}

// deprecated

func DeprecatedFlagRunImage(deprecatedRunImage *string) {
	flagSet.StringVar(deprecatedRunImage, "image", "", "reference to run image")
}

// helpers

func boolEnv(k string) bool {
	v := os.Getenv(k)
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}
