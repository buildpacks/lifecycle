package platform

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/str"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform/env"
)

// LifecycleInputs holds the values of command-line flags and args i.e., platform inputs to the lifecycle.
// Fields are the cumulative total of inputs across all lifecycle phases and all supported Platform APIs.
type LifecycleInputs struct {
	PlatformAPI           *api.Version
	AnalyzedPath          string
	AppDir                string
	BuildConfigDir        string
	BuildImageRef         string
	BuildpacksDir         string
	CacheDir              string
	CacheImageRef         string
	DefaultProcessType    string
	DeprecatedRunImageRef string
	ExtendKind            string
	ExtendedDir           string
	ExtensionsDir         string
	GeneratedDir          string
	GroupPath             string
	KanikoDir             string
	LaunchCacheDir        string
	LauncherPath          string
	LauncherSBOMDir       string
	LayersDir             string
	LayoutDir             string
	LogLevel              string
	OrderPath             string
	OutputImageRef        string
	PlanPath              string
	PlatformDir           string
	PreviousImageRef      string
	ProjectMetadataPath   string
	ReportPath            string
	RunImageRef           string
	RunPath               string
	StackPath             string
	UID                   int
	GID                   int
	ForceRebase           bool
	SkipLayers            bool
	UseDaemon             bool
	UseLayout             bool
	AdditionalTags        str.Slice // str.Slice satisfies the `Value` interface required by the `flag` package
	KanikoCacheTTL        time.Duration
}

const PlaceholderLayers = "<layers>"

// NewLifecycleInputs constructs new lifecycle inputs for the provided Platform API version.
// Inputs can be specified by the platform (in order of precedence) through:
//   - command-line flags
//   - environment variables
//   - falling back to the default value
//
// NewLifecycleInputs provides, for each input, the value from the environment if specified, falling back to the default.
// As the final value of the layers directory (if provided via the command-line) is not known,
// inputs that default to a child of the layers directory are provided with PlaceholderLayers as the layers directory.
// To be valid, inputs obtained from calling NewLifecycleInputs MUST be updated using UpdatePlaceholderPaths
// once the final value of the layers directory is known.
func NewLifecycleInputs(platformAPI *api.Version) *LifecycleInputs {
	var skipLayers bool
	if env.SkipLayers || env.SkipRestore {
		skipLayers = true
	}

	inputs := &LifecycleInputs{
		// Base Image

		UID: env.UID,
		GID: env.GID,

		// Builder Image

		BuildConfigDir: envOrDefault(env.BuildConfigDir, DefaultBuildConfigDir),
		BuildpacksDir:  envOrDefault(env.BuildpacksDir, DefaultBuildpacksDir),
		ExtensionsDir:  envOrDefault(env.ExtensionsDir, DefaultExtensionsDir),
		OrderPath:      envOrDefault(env.OrderPath, filepath.Join(PlaceholderLayers, DefaultOrderFile)), // we first look for order.toml in the layers directory, and fall back to /cnb/order.toml if it is not there
		RunPath:        envOrDefault(env.RunPath, DefaultRunPath),
		StackPath:      envOrDefault(env.StackPath, DefaultStackPath),

		// Platform

		// operator experience
		PlatformAPI: platformAPI,
		LogLevel:    envOrDefault(env.LogLevel, DefaultLogLevel),

		// dirs for detect/build
		AppDir:      envOrDefault(env.AppDir, DefaultAppDir),
		LayersDir:   envOrDefault(env.LayersDir, DefaultLayersDir),
		PlatformDir: envOrDefault(env.PlatformDir, DefaultPlatformDir),

		// data
		AnalyzedPath: envOrDefault(env.AnalyzedPath, filepath.Join(PlaceholderLayers, DefaultAnalyzedFile)),
		ExtendedDir:  envOrDefault(env.ExtendedDir, filepath.Join(PlaceholderLayers, DefaultExtendedDir)),
		GeneratedDir: envOrDefault(env.GeneratedDir, filepath.Join(PlaceholderLayers, DefaultGeneratedDir)),
		GroupPath:    envOrDefault(env.GroupPath, filepath.Join(PlaceholderLayers, DefaultGroupFile)),
		PlanPath:     envOrDefault(env.PlanPath, filepath.Join(PlaceholderLayers, DefaultPlanFile)),
		ReportPath:   envOrDefault(env.ReportPath, filepath.Join(PlaceholderLayers, DefaultReportFile)),

		// images
		BuildImageRef:         os.Getenv(env.VarBuildImage),
		PreviousImageRef:      os.Getenv(env.VarPreviousImage),
		RunImageRef:           os.Getenv(env.VarRunImage),
		DeprecatedRunImageRef: "", // no default

		// caching
		CacheDir:       os.Getenv(env.VarCacheDir),
		CacheImageRef:  os.Getenv(env.VarCacheImage),
		KanikoCacheTTL: timeEnvOrDefault(env.KanikoCacheTTL, DefaultKanikoCacheTTL),
		KanikoDir:      "/kaniko",
		LaunchCacheDir: os.Getenv(env.VarLaunchCacheDir),
		SkipLayers:     skipLayers,

		// export target
		AdditionalTags: nil, // no default
		OutputImageRef: "",  // no default
		UseDaemon:      env.UseDaemon,
		UseLayout:      env.UseLayout,
		LayoutDir:      os.Getenv(env.VarLayoutDir),

		// app image
		DefaultProcessType:  os.Getenv(env.VarProcessType),
		LauncherPath:        DefaultLauncherPath,
		LauncherSBOMDir:     DefaultBuildpacksioSBOMDir,
		ProjectMetadataPath: envOrDefault(env.ProjectMetadataPath, filepath.Join(PlaceholderLayers, DefaultProjectMetadataFile)),

		// image extension
		ExtendKind: envOrDefault(env.ExtendKind, DefaultExtendKind),

		// rebase
		ForceRebase: env.ForceRebase,
	}

	if platformAPI.LessThan("0.6") {
		// The default location for order.toml is /cnb/order.toml
		inputs.OrderPath = envOrDefault(env.OrderPath, CNBOrderPath)
	}

	if platformAPI.LessThan("0.5") {
		inputs.AnalyzedPath = envOrDefault(env.AnalyzedPath, DefaultAnalyzedFile)
		inputs.GeneratedDir = envOrDefault(env.GeneratedDir, DefaultGeneratedDir)
		inputs.GroupPath = envOrDefault(env.GroupPath, DefaultGroupFile)
		inputs.PlanPath = envOrDefault(env.PlanPath, DefaultPlanFile)
		inputs.ProjectMetadataPath = envOrDefault(env.ProjectMetadataPath, DefaultProjectMetadataFile)
		inputs.ReportPath = envOrDefault(env.ReportPath, DefaultReportFile)
	}

	return inputs
}

func (i *LifecycleInputs) DestinationImages() []string {
	var ret []string
	ret = appendOnce(ret, i.OutputImageRef)
	ret = appendOnce(ret, i.AdditionalTags...)
	return ret
}

func (i *LifecycleInputs) Images() []string {
	var ret []string
	ret = appendOnce(ret, i.DestinationImages()...)
	ret = appendOnce(ret, i.PreviousImageRef, i.BuildImageRef, i.RunImageRef, i.DeprecatedRunImageRef, i.CacheImageRef)
	return ret
}

func (i *LifecycleInputs) RegistryImages() []string {
	var ret []string
	ret = appendOnce(ret, i.CacheImageRef)
	if i.UseDaemon {
		return ret
	}
	ret = appendOnce(ret, i.Images()...)
	return ret
}

func appendOnce(list []string, els ...string) []string {
	for _, el := range els {
		if el == "" {
			continue
		}
		if notIn(list, el) {
			list = append(list, el)
		}
	}
	return list
}

func notIn(list []string, str string) bool {
	for _, el := range list {
		if el == str {
			return false
		}
	}
	return true
}

// shared helpers

func envOrDefault(envVal string, defaultVal string) string {
	if envVal != "" {
		return envVal
	}
	return defaultVal
}

func timeEnvOrDefault(envTTL time.Duration, defaultVal time.Duration) time.Duration {
	if envTTL != 0 {
		return envTTL
	}
	return defaultVal
}

// operations

func UpdatePlaceholderPaths(i *LifecycleInputs, _ log.Logger) error {
	toUpdate := i.placeholderPaths()
	for _, path := range toUpdate {
		if *path == "" {
			continue
		}
		if !isPlaceholder(*path) {
			continue
		}
		oldPath := *path
		toReplace := PlaceholderLayers
		if i.LayersDir == "" { // layers is unset when this call comes from the rebaser
			toReplace = PlaceholderLayers + string(filepath.Separator)
		}
		newPath := strings.Replace(*path, toReplace, i.LayersDir, 1)
		*path = newPath
		if isPlaceholderOrder(oldPath) {
			if _, err := os.Stat(newPath); err != nil {
				i.OrderPath = CNBOrderPath
			}
		}
	}
	return nil
}

func isPlaceholder(s string) bool {
	return strings.Contains(s, PlaceholderLayers)
}

func isPlaceholderOrder(s string) bool {
	return s == filepath.Join(PlaceholderLayers, DefaultOrderFile)
}

func (i *LifecycleInputs) placeholderPaths() []*string {
	return []*string{
		&i.AnalyzedPath,
		&i.ExtendedDir,
		&i.GeneratedDir,
		&i.GroupPath,
		&i.OrderPath,
		&i.PlanPath,
		&i.ProjectMetadataPath,
		&i.ReportPath,
	}
}

func ResolveAbsoluteDirPaths(i *LifecycleInputs, _ log.Logger) error {
	toUpdate := i.directoryPaths()
	for _, dir := range toUpdate {
		if *dir == "" {
			continue
		}
		abs, err := filepath.Abs(*dir)
		if err != nil {
			return err
		}
		*dir = abs
	}
	return nil
}

func (i *LifecycleInputs) directoryPaths() []*string {
	return []*string{
		&i.AppDir,
		&i.BuildConfigDir,
		&i.BuildpacksDir,
		&i.CacheDir,
		&i.ExtensionsDir,
		&i.GeneratedDir,
		&i.KanikoDir,
		&i.LaunchCacheDir,
		&i.LayersDir,
		&i.PlatformDir,
	}
}
