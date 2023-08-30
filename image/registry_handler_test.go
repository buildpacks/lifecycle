package image

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/lifecycle/testhelpers"
	testmockauth "github.com/buildpacks/lifecycle/testmock/auth"
)

func TestRegistryHandler(t *testing.T) {
	spec.Run(t, "RegistryHandler", testRegistryHandler, spec.Parallel(), spec.Report(report.Terminal{}))
}
func testRegistryHandler(t *testing.T, when spec.G, it spec.S) {
	when("insecure registry", func() {
		var (
			mockController *gomock.Controller
			mockKeychain   *testmockauth.MockKeychain
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			mockKeychain = testmockauth.NewMockKeychain(mockController)
		})

		it("returns WithRegistrySetting options for the domains specified", func() {
			registryHandler := NewRegistryHandler(mockKeychain, []string{"host.docker.internal"})

			registryOptions := registryHandler.GetInsecureRegistryOptions("host.docker.internal/bar")

			h.AssertEq(t, len(registryOptions), 1)
		})

		it("returns WithRegistrySetting options only for the domains specified", func() {
			registryHandler := NewRegistryHandler(mockKeychain, []string{"host.docker.internal", "this.is.just.a.try"})

			registryOptions := registryHandler.GetInsecureRegistryOptions("host.docker.internal/bar")

			h.AssertEq(t, len(registryOptions), 1)
		})

		it("returns empty options if any domain hasn't been specified and the imageRef is empty", func() {
			registryHandler := NewRegistryHandler(mockKeychain, nil)

			options := registryHandler.GetInsecureRegistryOptions("")

			h.AssertEq(t, len(options), 0)
		})

		it("returns empty options if an empty list of insecure registries has been passed but the imageRef has been passed anyway", func() {
			registryHandler := NewRegistryHandler(mockKeychain, []string{})

			options := registryHandler.GetInsecureRegistryOptions("host.docker.container")

			h.AssertEq(t, len(options), 0)
		})
	})
}
