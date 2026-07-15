package launch_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/launch"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDefaults(t *testing.T) {
	t.Run("values match the platform package", func(t *testing.T) {
		h.AssertEq(t, launch.EnvAppDir, platform.EnvAppDir)
		h.AssertEq(t, launch.EnvLayersDir, platform.EnvLayersDir)
		h.AssertEq(t, launch.EnvNoColor, platform.EnvNoColor)
		h.AssertEq(t, launch.EnvPlatformAPI, platform.EnvPlatformAPI)
		h.AssertEq(t, launch.EnvProcessType, platform.EnvProcessType)

		h.AssertEq(t, launch.DefaultPlatformAPI, platform.DefaultPlatformAPI)

		h.AssertEq(t, launch.DefaultAppDir, platform.DefaultAppDir)
		h.AssertEq(t, launch.DefaultLayersDir, platform.DefaultLayersDir)
	})
}
