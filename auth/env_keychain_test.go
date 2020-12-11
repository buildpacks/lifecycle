package auth_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/auth"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestEnvKeychain(t *testing.T) {
	spec.Run(t, "ResolveKeychain", testEnvKeychain, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testEnvKeychain(t *testing.T, when spec.G, it spec.S) {
	when("ResolveKeychain", func() {
		when("#Resolve", func() {
			it.Before(func() {
				// set CNB_REGISTRY_AUTH
				err := os.Setenv(
					"CNB_REGISTRY_AUTH",
					`{"basic-registry.com": "Basic some-basic-auth=", "bearer-registry.com": "Bearer some-bearer-auth="}`,
				)
				h.AssertNil(t, err)
			})

			it.After(func() {
				h.AssertNil(t, os.Unsetenv("CNB_REGISTRY_AUTH"))
			})

			when("valid auth env variable is set", func() {
				when("valid variable", func() {
					it("loads the basic auth from memory", func() {
						resolvedKeychain, err := auth.ResolveKeychain("CNB_REGISTRY_AUTH")
						h.AssertNil(t, err)
						h.AssertNil(t, os.Unsetenv("CNB_REGISTRY_AUTH")) // unset variable to prove that our test is using a pre-resolved keychain

						registry, err := name.NewRegistry("basic-registry.com", name.WeakValidation)
						h.AssertNil(t, err)

						authenticator, err := resolvedKeychain.Resolve(registry)
						h.AssertNil(t, err)

						header, err := authenticator.Authorization()
						h.AssertNil(t, err)

						h.AssertEq(t, header, &authn.AuthConfig{Auth: "some-basic-auth="})
					})

					it("loads the bearer auth from memory", func() {
						resolvedKeychain, err := auth.ResolveKeychain("CNB_REGISTRY_AUTH")
						h.AssertNil(t, err)
						h.AssertNil(t, os.Unsetenv("CNB_REGISTRY_AUTH")) // unset variable to prove that our test is using a pre-resolved keychain

						registry, err := name.NewRegistry("bearer-registry.com", name.WeakValidation)
						h.AssertNil(t, err)

						authenticator, err := resolvedKeychain.Resolve(registry)
						h.AssertNil(t, err)

						header, err := authenticator.Authorization()
						h.AssertNil(t, err)

						h.AssertEq(t, header, &authn.AuthConfig{RegistryToken: "some-bearer-auth="})
					})

					when("the environment does not have an auth header", func() {
						it("uses the fallback keychain", func() {
							assertUsesFallbackKeychain(t)
						})

						when("the fallback keychain does not have an auth header", func() {
							it("returns an Anonymous authenticator", func() {
								assertFallbackKeychainWithoutHeaderReturnsAnonymous(t)
							})
						})

						when("the fallback keychain cannot be resolved", func() {
							it("returns an Anonymous authenticator", func() {
								assertFallbackKeychainErrorReturnsAnonymous(t)
							})
						})
					})
				})
			})

			when("invalid auth env variable is set", func() {
				it.Before(func() {
					err := os.Setenv("CNB_REGISTRY_AUTH", "NOT -- JSON")
					h.AssertNil(t, err)
				})

				it("errors", func() {
					_, err := auth.ResolveKeychain("CNB_REGISTRY_AUTH", auth.WithImages("some-registry.com/some-image"))
					h.AssertNotNil(t, err)
					h.AssertStringContains(t, err.Error(), "failed to parse")
				})
			})

			when("auth env variable is not set", func() {
				it.Before(func() {
					h.AssertNil(t, os.Unsetenv("CNB_REGISTRY_AUTH"))
				})

				it("uses the fallback keychain", func() {
					assertUsesFallbackKeychain(t)
				})

				when("the fallback keychain does not have an auth header", func() {
					it("returns an Anonymous authenticator", func() {
						assertFallbackKeychainWithoutHeaderReturnsAnonymous(t)
					})
				})

				when("the fallback keychain cannot be resolved", func() {
					it("returns an Anonymous authenticator", func() {
						assertFallbackKeychainErrorReturnsAnonymous(t)
					})
				})
			})
		})
	})

	when("#BuildEnvVar", func() {
		var keychain authn.Keychain

		it.Before(func() {
			keychain = &FakeKeychain{
				authMap: map[string]*authn.AuthConfig{
					"some-registry.com": {
						Username: "user",
						Password: "password",
					},
					"other-registry.com": {
						Auth: "asdf=",
					},
					"index.docker.io": {
						RegistryToken: "qwerty=",
					},
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

			var jsonAuth bytes.Buffer
			h.AssertNil(t, json.Compact(&jsonAuth, []byte(`{
	"index.docker.io": "Bearer qwerty=",
	"other-registry.com": "Basic asdf=",
	"some-registry.com": "Basic dXNlcjpwYXNzd29yZA=="
}`)))
			h.AssertEq(t, envVar, jsonAuth.String())
		})

		it("returns an empty result for Anonymous registries", func() {
			envVar, err := auth.BuildEnvVar(keychain, "anonymous.com/dockerhub/image")
			h.AssertNil(t, err)

			h.AssertEq(t, envVar, "{}")
		})
	})
}

type FakeKeychain struct {
	authMap           map[string]*authn.AuthConfig
	returnsForResolve error // if set, return the error
}

func (f *FakeKeychain) Resolve(r authn.Resource) (authn.Authenticator, error) {
	if f.returnsForResolve != nil {
		return nil, f.returnsForResolve
	}

	key, ok := f.authMap[r.RegistryStr()]
	if ok {
		return &providedAuth{config: key}, nil
	}

	return authn.Anonymous, nil
}

type providedAuth struct {
	config *authn.AuthConfig
}

func (p *providedAuth) Authorization() (*authn.AuthConfig, error) {
	return p.config, nil
}

func assertUsesFallbackKeychain(t *testing.T) {
	fakeKeychain := FakeKeychain{authMap: map[string]*authn.AuthConfig{
		"no-env-auth-registry.com": {
			Auth: "asdf=",
		},
	}}

	resolvedKeychain, err := auth.ResolveKeychain(
		"CNB_REGISTRY_AUTH",
		auth.WithImages("no-env-auth-registry.com/some-image"),
		auth.WithFallbackKeychain(&fakeKeychain),
	)
	h.AssertNil(t, err)
	h.AssertNil(t, os.Unsetenv("CNB_REGISTRY_AUTH")) // unset variable to prove that our test is using a pre-resolved keychain

	registry, err := name.NewRegistry("no-env-auth-registry.com", name.WeakValidation)
	h.AssertNil(t, err)

	authenticator, err := resolvedKeychain.Resolve(registry)
	h.AssertNil(t, err)

	header, err := authenticator.Authorization()
	h.AssertNil(t, err)

	h.AssertEq(t, header, &authn.AuthConfig{Auth: "asdf="})
}

func assertFallbackKeychainWithoutHeaderReturnsAnonymous(t *testing.T) {
	fakeKeychain := FakeKeychain{authMap: map[string]*authn.AuthConfig{}} // empty map

	resolvedKeychain, err := auth.ResolveKeychain(
		"CNB_REGISTRY_AUTH",
		auth.WithImages("no-auth-registry.com/some-image"),
		auth.WithFallbackKeychain(&fakeKeychain),
	)
	h.AssertNil(t, err)
	h.AssertNil(t, os.Unsetenv("CNB_REGISTRY_AUTH")) // unset variable to prove that our test is using a pre-resolved keychain

	registry, err := name.NewRegistry("no-auth-registry.com", name.WeakValidation)
	h.AssertNil(t, err)

	authenticator, err := resolvedKeychain.Resolve(registry)
	h.AssertNil(t, err)

	_, err = authenticator.Authorization()
	h.AssertNil(t, err)

	h.AssertEq(t, authenticator, authn.Anonymous)
}

func assertFallbackKeychainErrorReturnsAnonymous(t *testing.T) {
	fakeKeychain := FakeKeychain{returnsForResolve: errors.New("some-error")}

	resolvedKeychain, err := auth.ResolveKeychain(
		"CNB_REGISTRY_AUTH",
		auth.WithImages("some-registry.com/some-image"),
		auth.WithFallbackKeychain(&fakeKeychain),
	)
	h.AssertNil(t, err)
	h.AssertNil(t, os.Unsetenv("CNB_REGISTRY_AUTH")) // unset variable to prove that our test is using a pre-resolved keychain

	registry, err := name.NewRegistry("some-registry.com", name.WeakValidation)
	h.AssertNil(t, err)

	authenticator, err := resolvedKeychain.Resolve(registry)
	h.AssertNil(t, err)

	_, err = authenticator.Authorization()
	h.AssertNil(t, err)

	h.AssertEq(t, authenticator, authn.Anonymous)
}
