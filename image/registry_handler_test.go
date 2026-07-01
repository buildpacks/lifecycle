package image

import (
	"testing"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestRegistryHandler(t *testing.T) {
	t.Parallel()
	t.Run("insecure registry", func(t *testing.T) {
		t.Run("returns WithRegistrySetting options for the domain specified", func(t *testing.T) {
			registryOptions := GetInsecureOptions([]string{"host.docker.internal"})

			h.AssertEq(t, len(registryOptions), 1)
		})
		t.Run("returns multiple WithRegistrySetting options for the domains specified", func(t *testing.T) {
			registryOptions := GetInsecureOptions([]string{"host.docker.internal", "another.docker.internal"})

			h.AssertEq(t, len(registryOptions), 2)
		})
		t.Run("returns empty options if any domain hasn't been specified", func(t *testing.T) {
			options := GetInsecureOptions(nil)

			h.AssertEq(t, len(options), 0)
		})
		t.Run("returns empty options if an empty list of insecure registries has been passed", func(t *testing.T) {
			options := GetInsecureOptions([]string{})

			h.AssertEq(t, len(options), 0)
		})
	})
}
