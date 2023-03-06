package platform

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/internal/str"
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
	SkipLayers            bool
	UseDaemon             bool
	UseLayout             bool
	AdditionalTags        str.Slice // str.Slice satisfies the `Value` interface required by the `flag` package
	KanikoCacheTTL        time.Duration
}

func NewLifecycleInputs(platformAPI *api.Version, layersDir string) *LifecycleInputs {
	var skipLayers bool
	if boolEnv(EnvSkipLayers) || boolEnv(EnvSkipRestore) {
		skipLayers = true
	}

	// FIXME: api compatibility should be validated here
	if layersDir == "" {
		layersDir = envOrDefault(EnvLayersDir, DefaultLayersDir)
	}

	orderPath := filepath.Join(layersDir, DefaultOrderFile)
	if _, err := os.Stat(orderPath); err != nil {
		orderPath = DefaultOrderPath
	}

	inputs := &LifecycleInputs{
		// Operator config

		LogLevel:    envOrDefault(EnvLogLevel, DefaultLogLevel),
		PlatformAPI: platformAPI,
		ExtendKind:  envOrDefault(EnvExtendKind, DefaultExtendKind),
		UseDaemon:   boolEnv(EnvUseDaemon),
		UseLayout:   boolEnv(EnvUseLayout),

		// Provided by the base image

		UID: intEnv(EnvUID),
		GID: intEnv(EnvGID),

		// Provided by the builder image

		BuildConfigDir: envOrDefault(EnvBuildConfigDir, DefaultBuildConfigDir),
		BuildpacksDir:  envOrDefault(EnvBuildpacksDir, DefaultBuildpacksDir),
		ExtensionsDir:  envOrDefault(EnvExtensionsDir, DefaultExtensionsDir),
		RunPath:        envOrDefault(EnvRunPath, DefaultRunPath),
		StackPath:      envOrDefault(EnvStackPath, DefaultStackPath),

		// Provided at build time

		AppDir:      envOrDefault(EnvAppDir, DefaultAppDir),
		LayersDir:   layersDir,
		LayoutDir:   os.Getenv(EnvLayoutDir),
		OrderPath:   orderPath,
		PlatformDir: envOrDefault(EnvPlatformDir, DefaultPlatformDir),

		// The following instruct the lifecycle where to write files and data during the build

		AnalyzedPath: envOrDefault(EnvAnalyzedPath, filepath.Join(layersDir, DefaultAnalyzedFile)),
		GeneratedDir: envOrDefault(EnvGeneratedDir, filepath.Join(layersDir, DefaultGeneratedDir)),
		ExtendedDir:  envOrDefault(EnvExtendedDir, filepath.Join(layersDir, DefaultExtendedDir)), // TODO: add test
		GroupPath:    envOrDefault(EnvGroupPath, filepath.Join(layersDir, DefaultGroupFile)),
		PlanPath:     envOrDefault(EnvPlanPath, filepath.Join(layersDir, DefaultPlanFile)),
		ReportPath:   envOrDefault(EnvReportPath, filepath.Join(layersDir, DefaultReportFile)),

		// Configuration options with respect to caching

		CacheDir:       os.Getenv(EnvCacheDir),
		CacheImageRef:  os.Getenv(EnvCacheImage),
		KanikoCacheTTL: timeEnvOrDefault(EnvKanikoCacheTTL, DefaultKanikoCacheTTL),
		KanikoDir:      "/kaniko",
		LaunchCacheDir: os.Getenv(EnvLaunchCacheDir),
		SkipLayers:     skipLayers,

		// Images used by the lifecycle during the build

		AdditionalTags:        nil, // no default
		BuildImageRef:         os.Getenv(EnvBuildImage),
		DeprecatedRunImageRef: "", // no default
		OutputImageRef:        "", // no default
		PreviousImageRef:      os.Getenv(EnvPreviousImage),
		RunImageRef:           os.Getenv(EnvRunImage),

		// Configuration options for the output application image

		DefaultProcessType:  os.Getenv(EnvProcessType),
		LauncherPath:        DefaultLauncherPath,
		LauncherSBOMDir:     DefaultBuildpacksioSBOMDir,
		ProjectMetadataPath: envOrDefault(EnvProjectMetadataPath, filepath.Join(layersDir, DefaultProjectMetadataFile)),
	}

	if platformAPI.LessThan("0.5") {
		inputs.AnalyzedPath = envOrDefault(EnvAnalyzedPath, DefaultAnalyzedFile)
		inputs.GeneratedDir = envOrDefault(EnvGeneratedDir, DefaultGeneratedDir)
		inputs.GroupPath = envOrDefault(EnvGroupPath, DefaultGroupFile)
		inputs.PlanPath = envOrDefault(EnvPlanPath, DefaultPlanFile)
		inputs.ProjectMetadataPath = envOrDefault(EnvProjectMetadataPath, DefaultProjectMetadataFile)
		inputs.ReportPath = envOrDefault(EnvReportPath, DefaultReportFile)
	}

	if platformAPI.LessThan("0.6") {
		// The default location for order.toml is /cnb/order.toml
		inputs.OrderPath = envOrDefault(EnvOrderPath, DefaultOrderPath)
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

func timeEnvOrDefault(key string, defaultVal time.Duration) time.Duration {
	envTTL := os.Getenv(key)
	if envTTL == "" {
		return defaultVal
	}
	ttl, err := time.ParseDuration(envTTL)
	if err != nil {
		return defaultVal
	}
	return ttl
}
