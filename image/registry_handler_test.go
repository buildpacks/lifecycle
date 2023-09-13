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
			registryOptions := GetInsecureOptions([]string{"host.docker.internal"}, "host.docker.internal/bar")

			h.AssertEq(t, len(registryOptions), 1)
		})

		it("returns WithRegistrySetting options only for the domains specified", func() {
			registryOptions := GetInsecureOptions([]string{"host.docker.internal", "this.is.just.a.try"}, "host.docker.internal/bar")

			h.AssertEq(t, len(registryOptions), 1)
		})

		it("returns empty options if any domain hasn't been specified and the imageRef is empty", func() {
			options := GetInsecureOptions(nil, "")

			h.AssertEq(t, len(options), 0)
		})

		it("returns empty options if an empty list of insecure registries has been passed but the imageRef has been passed anyway", func() {
			options := GetInsecureOptions([]string{}, "host.docker.container")

			h.AssertEq(t, len(options), 0)
		})
	})
}
