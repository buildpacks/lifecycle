package auth

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestAzureKeychain(t *testing.T) {
	spec.Run(t, "acrHostnameGuardedKeychain", testAzureKeychain, spec.Report(report.Terminal{}))
}

func testAzureKeychain(t *testing.T, when spec.G, it spec.S) {
	when("#Resolve", func() {
		var (
			resolved  bool
			keychain  *acrHostnameGuardedKeychain
			fakeInner *fakeResolverKeychain
		)

		it.Before(func() {
			resolved = false
			fakeInner = &fakeResolverKeychain{onResolve: func() { resolved = true }}
			keychain = &acrHostnameGuardedKeychain{keychain: fakeInner}
		})

		when("the hostname is a genuine Azure Container Registry host", func() {
			it("forwards resolution to the underlying keychain", func() {
				for _, hostname := range []string{
					"myregistry.azurecr.io",
					"myregistry.azurecr.cn",
					"myregistry.azurecr.de",
					"myregistry.azurecr.us",
					"mcr.microsoft.com",
				} {
					resolved = false
					registry, err := name.NewRegistry(hostname, name.WeakValidation)
					h.AssertNil(t, err)

					_, err = keychain.Resolve(registry)
					h.AssertNil(t, err)
					h.AssertEq(t, resolved, true)
				}
			})
		})

		when("the hostname merely contains an ACR-like substring", func() {
			it("does not forward resolution to the underlying keychain (GO-2026-6225)", func() {
				for _, hostname := range []string{
					"myregistry.azurecr.io.attacker.com",
					"azurecr.io.attacker.com",
					"notazurecr.io",
				} {
					resolved = false
					registry, err := name.NewRegistry(hostname, name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := keychain.Resolve(registry)
					h.AssertNil(t, err)
					h.AssertEq(t, resolved, false)
					h.AssertEq(t, authenticator, authn.Anonymous)
				}
			})
		})
	})
}

type fakeResolverKeychain struct {
	onResolve func()
}

func (k *fakeResolverKeychain) Resolve(_ authn.Resource) (authn.Authenticator, error) {
	k.onResolve()
	return authn.Anonymous, nil
}
