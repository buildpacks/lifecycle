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

func TestEnvKeychain(t *testing.T) {
	spec.Run(t, "Env Keychain", testEnvKeychain, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testEnvKeychain(t *testing.T, when spec.G, it spec.S) {
	when("EnvKeychain", func() {
		var envKeyChain authn.Keychain

		it.Before(func() {
			envKeyChain = &auth.EnvKeychain{}
		})

		it.After(func() {
			err := os.Unsetenv("CNB_REGISTRY_AUTH")
			h.AssertNil(t, err)
		})

		when("#Resolve", func() {
			when("valid auth env variable is set", func() {
				it.Before(func() {
					err := os.Setenv("CNB_REGISTRY_AUTH", "{\"some-registry.com\": \"some-auth-header\"}")
					h.AssertNil(t, err)
				})

				it("loads the auth from the environment", func() {
					registry, err := name.NewRegistry("some-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					auth, err := envKeyChain.Resolve(registry)
					h.AssertNil(t, err)

					header, err := auth.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, header, "some-auth-header")
				})

				it("returns an Anonymous authenticator when the environment does not have a auth header", func() {
					registry, err := name.NewRegistry("no-env-auth-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					auth, err := envKeyChain.Resolve(registry)
					h.AssertNil(t, err)

					h.AssertEq(t, auth, authn.Anonymous)
				})
			})

			when("invalid env var is set", func() {
				it.Before(func() {
					err := os.Setenv("CNB_REGISTRY_AUTH", "NOT -- JSON")
					h.AssertNil(t, err)
				})

				it("returns an error", func() {
					registry, err := name.NewRegistry("some-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					_, err = envKeyChain.Resolve(registry)
					h.AssertError(t, err, "failed to parse CNB_REGISTRY_AUTH value")
				})
			})

			when("env var is not set", func() {
				it("returns an Anonymous authenticator", func() {
					registry, err := name.NewRegistry("no-env-auth-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					auth, err := envKeyChain.Resolve(registry)
					h.AssertNil(t, err)

					h.AssertEq(t, auth, authn.Anonymous)
				})
			})
		})
	})

	when("#BuildAuthEnvVar", func() {
		var keychain authn.Keychain

		it.Before(func() {
			keychain = &FakeKeychain{
				auths: map[string]string{
					"some-registry.com":  "some-registry.com-auth",
					"other-registry.com": "other-registry.com-auth",
					"index.docker.io":    "dockerhub-auth",
				},
			}
		})

		it("builds json encoded env with auth headers", func() {
			envVar, err := auth.BuildEnvVar(keychain,
				"some-registry.com/image",
				"some-registry.com/image2",
				"other-registry.com/image3",
				"my/image")
			h.AssertNil(t, err)

			h.AssertEq(t, envVar, "{\"index.docker.io\":\"dockerhub-auth\",\"other-registry.com\":\"other-registry.com-auth\",\"some-registry.com\":\"some-registry.com-auth\"}")
		})

		it("returns an empty result for Anonymous registries", func() {
			envVar, err := auth.BuildEnvVar(keychain, "anonymous.com/dockerhub/image")
			h.AssertNil(t, err)

			h.AssertEq(t, envVar, "{}")
		})
	})
}

type FakeKeychain struct {
	auths map[string]string
}

func (f *FakeKeychain) Resolve(r name.Registry) (authn.Authenticator, error) {
	key, ok := f.auths[r.Name()]
	if ok {
		return &providedAuth{auth: key}, nil
	}

	return authn.Anonymous, nil
}

type providedAuth struct {
	auth string
}

func (p *providedAuth) Authorization() (string, error) {
	return p.auth, nil
}
