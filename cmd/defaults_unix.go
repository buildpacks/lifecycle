// +build linux darwin

package cmd

const (
	DefaultLayersDir           = "/layers"
	DefaultAppDir              = "/workspace"
	DefaultBuildpacksDir       = "/cnb/buildpacks"
	DefaultPlatformDir         = "/platform"
	DefaultOrderPath           = "/cnb/order.toml"
	DefaultGroupPath           = "./group.toml"
	DefaultStackPath           = "/cnb/stack.toml"
	DefaultAnalyzedPath        = "./analyzed.toml"
	DefaultPlanPath            = "./plan.toml"
	DefaultProcessType         = "web"
	DefaultLauncherPath        = "/cnb/lifecycle/launcher"
	DefaultLogLevel            = "info"
	DefaultProjectMetadataPath = "./project-metadata.toml"
)
