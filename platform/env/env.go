package env

import (
	"os"
	"strconv"
	"time"
)

var (
	AnalyzedPath        = func() string { return os.Getenv(VarAnalyzedPath) }()
	AppDir              = func() string { return os.Getenv(VarAppDir) }()
	BuildConfigDir      = func() string { return os.Getenv(VarBuildConfigDir) }()
	BuildImage          = func() string { return os.Getenv(VarBuildImage) }()
	BuildpacksDir       = func() string { return os.Getenv(VarBuildpacksDir) }()
	CacheDir            = func() string { return os.Getenv(VarCacheDir) }()
	CacheImage          = func() string { return os.Getenv(VarCacheImage) }()
	DeprecationMode     = func() string { return os.Getenv(VarDeprecationMode) }()
	ExperimentalMode    = func() string { return os.Getenv(VarExperimentalMode) }()
	ExtendKind          = func() string { return os.Getenv(VarExtendKind) }()
	ExtendedDir         = func() string { return os.Getenv(VarExtendedDir) }()
	ExtensionsDir       = func() string { return os.Getenv(VarExtensionsDir) }()
	ForceRebase         = func() bool { return boolEnv(VarForceRebase) }()
	GID                 = func() int { return intEnv(VarGID) }()
	GeneratedDir        = func() string { return os.Getenv(VarGeneratedDir) }()
	GroupPath           = func() string { return os.Getenv(VarGroupPath) }()
	KanikoCacheTTL      = func() time.Duration { return timeEnv(VarKanikoCacheTTL) }()
	LaunchCacheDir      = func() string { return os.Getenv(VarLaunchCacheDir) }()
	LayersDir           = func() string { return os.Getenv(VarLayersDir) }()
	LayoutDir           = func() string { return os.Getenv(VarLayoutDir) }()
	LogLevel            = func() string { return os.Getenv(VarLogLevel) }()
	NoColor             = func() bool { return boolEnv(VarNoColor) }()
	OrderPath           = func() string { return os.Getenv(VarOrderPath) }()
	PlanPath            = func() string { return os.Getenv(VarPlanPath) }()
	PlatformAPI         = func() string { return os.Getenv(VarPlatformAPI) }()
	PlatformDir         = func() string { return os.Getenv(VarPlatformDir) }()
	PreviousImage       = func() string { return os.Getenv(VarPreviousImage) }()
	ProcessType         = func() string { return os.Getenv(VarProcessType) }()
	ProjectMetadataPath = func() string { return os.Getenv(VarProjectMetadataPath) }()
	RegistryAuth        = func() string { return os.Getenv(VarRegistryAuth) }()
	ReportPath          = func() string { return os.Getenv(VarReportPath) }()
	RunImage            = func() string { return os.Getenv(VarRunImage) }()
	RunPath             = func() string { return os.Getenv(VarRunPath) }()
	SkipLayers          = func() bool { return boolEnv(VarSkipLayers) }()
	SkipRestore         = func() bool { return boolEnv(VarSkipRestore) }()
	StackPath           = func() string { return os.Getenv(VarStackPath) }()
	UID                 = func() int { return intEnv(VarUID) }()
	UseDaemon           = func() bool { return boolEnv(VarUseDaemon) }()
	UseLayout           = func() bool { return boolEnv(VarUseLayout) }()
	DeprecatedStackID   = func() string { return os.Getenv(DeprecatedVarStackID) }()
)

func boolEnv(k string) bool {
	v := os.Getenv(k)
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

func intEnv(k string) int {
	v := os.Getenv(k)
	d, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return d
}

func timeEnv(key string) time.Duration {
	envTTL := os.Getenv(key)
	if envTTL == "" {
		return 0
	}
	ttl, err := time.ParseDuration(envTTL)
	if err != nil {
		return 0
	}
	return ttl
}
