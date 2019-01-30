package auth_test

import (
	"os"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle/image/auth"
	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestLegacyEnvKeychain(t *testing.T) {
	spec.Run(t, "Legacy Env Keychain", testLegacyEnvKeychain, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testLegacyEnvKeychain(t *testing.T, when spec.G, it spec.S) {
	when("LegacyEnvKeychain", func() {
		var legacyEnvKeyChain authn.Keychain

		it.Before(func() {
			legacyEnvKeyChain = &auth.LegacyEnvKeychain{}
		})

		it.After(func() {
			err := os.Unsetenv("PACK_REGISTRY_AUTH")
			h.AssertNil(t, err)
		})

		when("#Resolve", func() {
			when("valid auth env variable is set", func() {
				it.Before(func() {
					err := os.Setenv("PACK_REGISTRY_AUTH", "some-auth-header")
					h.AssertNil(t, err)
				})

				it("loads the auth from the environment", func() {
					registry, err := name.NewRegistry("some-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					auth, err := legacyEnvKeyChain.Resolve(registry)
					h.AssertNil(t, err)

					header, err := auth.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, header, "some-auth-header")
				})
			})

			when("env var is not set", func() {
				it("returns an Anonymous authenticator", func() {
					registry, err := name.NewRegistry("no-env-auth-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					auth, err := legacyEnvKeyChain.Resolve(registry)
					h.AssertNil(t, err)

					h.AssertEq(t, auth, authn.Anonymous)
				})
			})
		})
	})
}
