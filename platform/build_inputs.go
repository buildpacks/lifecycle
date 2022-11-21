package platform

import (
	"github.com/buildpacks/lifecycle/api"
)

// DefaultBuildInputs accepts a Platform API version and returns a set of lifecycle inputs
//   with default values filled in for the `build` phase.
func DefaultBuildInputs(platformAPI *api.Version) LifecycleInputs {
	var inputs LifecycleInputs
	switch {
	case platformAPI.AtLeast("0.5"):
		inputs = defaultBuildInputs()
	default:
		inputs = defaultBuildInputs03To04()
	}
	inputs.PlatformAPI = platformAPI
	return inputs
}

func defaultBuildInputs() LifecycleInputs {
	bi := defaultBuildInputs03To04()
	bi.GroupPath = envOrDefault(EnvGroupPath, placeholderGroupPath)
	bi.PlanPath = envOrDefault(EnvPlanPath, placeholderPlanPath)
	return bi
}

func defaultBuildInputs03To04() LifecycleInputs {
	return LifecycleInputs{
		AppDir:        envOrDefault(EnvAppDir, DefaultAppDir),               // <app>
		BuildpacksDir: envOrDefault(EnvBuildpacksDir, DefaultBuildpacksDir), // <buildpacks>
		GroupPath:     envOrDefault(EnvGroupPath, DefaultGroupFile),         // <group>
		LayersDir:     envOrDefault(EnvLayersDir, DefaultLayersDir),         // <layers>
		LogLevel:      envOrDefault(EnvLogLevel, DefaultLogLevel),           // <log-level>
		PlanPath:      envOrDefault(EnvPlanPath, DefaultPlanFile),           // <plan>
		PlatformDir:   envOrDefault(EnvPlatformDir, DefaultPlatformDir),     // <platform>
	}
}
