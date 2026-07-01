package platform_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestPlatform(t *testing.T) {
	t.Parallel()
	for _, platformAPI := range api.Platform.Supported {
		platformAPI := platformAPI
		t.Run("unit-platform/"+platformAPI.String(), func(t *testing.T) {
			t.Parallel()
			t.Run("#NewPlatformFor", func(t *testing.T) {
				t.Run("configures the platform", func(t *testing.T) {
					foundPlatform := platform.NewPlatformFor(platformAPI.String())

					t.Log("with a default exiter")
					_, ok := foundPlatform.Exiter.(*platform.DefaultExiter)
					h.AssertEq(t, ok, true)

					t.Log("with an api")
					h.AssertEq(t, foundPlatform.API(), platformAPI)
				})
			})
		})
	}
}
