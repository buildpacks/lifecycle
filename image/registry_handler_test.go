package image

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestRegistryHandler(t *testing.T) {
	spec.Run(t, "RegistryHandler", testRegistryHandler, spec.Parallel(), spec.Report(report.Terminal{}))
}
func testRegistryHandler(t *testing.T, when spec.G, it spec.S) {
	when("insecure registry", func() {
		it("returns WithRegistrySetting options for the domains specified", func() {
			registryOptions := GetInsecureOptions([]string{"host.docker.internal"})

			h.AssertEq(t, len(registryOptions), 1)
		})

		it("returns empty options if any domain hasn't been specified", func() {
			options := GetInsecureOptions(nil)

			h.AssertEq(t, len(options), 0)
		})

		it("returns empty options if an empty list of insecure registries has been passed", func() {
			options := GetInsecureOptions([]string{})

			h.AssertEq(t, len(options), 0)
		})
	})
}
