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
	spec.Run(t, "Keychain", testEnvKeychain, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testEnvKeychain(t *testing.T, when spec.G, it spec.S) {
	when("#NewEnvKeychain", func() {
		when("environment variable is set", func() {
			when("valid", func() {
				it.Before(func() {
					err := os.Setenv(
						"CNB_REGISTRY_AUTH",
						`{
	"basic-registry.com": "Basic some-basic-auth=",
	"bearer-registry.com": "Bearer some-bearer-auth=",
	"oauth.registry.io": "X-Identity some-identity-token="
}`,
					)
					h.AssertNil(t, err)
				})

				it.After(func() {
					h.AssertNil(t, os.Unsetenv("CNB_REGISTRY_AUTH"))
				})

				it("returns a resolved keychain from the environment", func() {
					keychain, err := auth.NewEnvKeychain("CNB_REGISTRY_AUTH")
					h.AssertNil(t, err)

					h.AssertEq(t, keychain, &auth.EnvKeychain{
						AuthHeaders: map[string]string{
							"basic-registry.com":  "Basic some-basic-auth=",
							"bearer-registry.com": "Bearer some-bearer-auth=",
							"oauth.registry.io":   "X-Identity some-identity-token=",
						},
					})
				})
			})

			when("invalid", func() {
				it.Before(func() {
					err := os.Setenv("CNB_REGISTRY_AUTH", "NOT -- JSON")
					h.AssertNil(t, err)
				})

				it.After(func() {
					h.AssertNil(t, os.Unsetenv("CNB_REGISTRY_AUTH"))
				})

				it("returns an error", func() {
					_, err := auth.NewEnvKeychain("CNB_REGISTRY_AUTH")
					h.AssertNotNil(t, err)
				})
			})
		})

		when("environment variable is not set", func() {
			it("returns an empty keychain", func() {
				keychain, err := auth.NewEnvKeychain("CNB_REGISTRY_AUTH")
				h.AssertNil(t, err)

				h.AssertEq(t, keychain, &auth.EnvKeychain{
					AuthHeaders: map[string]string{},
				})
			})
		})
	})

	when("EnvKeychain", func() {
		when("#Resolve", func() {
			var envKeychain auth.EnvKeychain

			it.Before(func() {
				envKeychain = auth.EnvKeychain{AuthHeaders: map[string]string{
					"basic-registry.com":  "Basic some-basic-auth=",
					"bearer-registry.com": "Bearer some-bearer-auth=",
					"oauth.registry.io":   "X-Identity some-identity-token=",
					"bad-header.com":      "Some Bad Header",
				}}
			})

			when("auth header is found", func() {
				it("loads the basic auth from memory", func() {
					registry, err := name.NewRegistry("basic-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := envKeychain.Resolve(registry)
					h.AssertNil(t, err)

					header, err := authenticator.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, header, &authn.AuthConfig{Auth: "some-basic-auth="})
				})

				it("loads the bearer auth from memory", func() {
					registry, err := name.NewRegistry("bearer-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := envKeychain.Resolve(registry)
					h.AssertNil(t, err)

					header, err := authenticator.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, header, &authn.AuthConfig{RegistryToken: "some-bearer-auth="})
				})

				it("loads the identity token from memory", func() {
					registry, err := name.NewRegistry("oauth.registry.io", name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := envKeychain.Resolve(registry)
					h.AssertNil(t, err)

					header, err := authenticator.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, header, &authn.AuthConfig{IdentityToken: "some-identity-token="})
				})

				when("error parsing header", func() {
					it("doesn't print the header in the error message", func() {
						registry, err := name.NewRegistry("bad-header.com", name.WeakValidation)
						h.AssertNil(t, err)

						_, err = envKeychain.Resolve(registry)
						h.AssertNotNil(t, err)
						h.AssertStringContains(t, err.Error(), "parsing auth header")
						h.AssertStringDoesNotContain(t, err.Error(), "Some Bad Header")
					})
				})
			})

			when("auth header is not found", func() {
				it("returns an Anonymous authenticator", func() {
					registry, err := name.NewRegistry("no-auth-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := envKeychain.Resolve(registry)
					h.AssertNil(t, err)

					_, err = authenticator.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, authenticator, authn.Anonymous)
				})
			})

			when("auth header is empty", func() {
				it.Before(func() {
					envKeychain = auth.EnvKeychain{AuthHeaders: map[string]string{}}
				})

				it("returns an Anonymous authenticator", func() {
					registry, err := name.NewRegistry("some-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := envKeychain.Resolve(registry)
					h.AssertNil(t, err)

					_, err = authenticator.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, authenticator, authn.Anonymous)
				})
			})
		})
	})

	when("#NewResolvedKeychain", func() {
		it("returns a resolved keychain from the provided keychain", func() {
			keychain := &FakeKeychain{
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
					"oauth.registry.io": {
						IdentityToken: "hjkl=",
					},
					"empty-auth.registry.io": {},
				},
			}

			resolvedKeychain := auth.NewResolvedKeychain(
				keychain,
				"some-registry.com/image1",
				"some-registry.com/image2",
				"", // empty strings should be ignored
				"other-registry.com/image",
				"my/image", // index.docker.io
				"oauth.registry.io/image",
				"empty-auth.registry.io/image",
			)

			h.AssertEq(t, resolvedKeychain, &auth.ResolvedKeychain{
				AuthConfigs: map[string]*authn.AuthConfig{
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
					"oauth.registry.io": {
						IdentityToken: "hjkl=",
					},
				},
			})
		})

		when("the provided keychain fails to resolve", func() {
			it("returns an empty keychain", func() {
				keychain := &FakeKeychain{
					returnsForResolve: errors.New("some-error"),
				}

				resolvedKeychain := auth.NewResolvedKeychain(keychain)

				h.AssertEq(t, resolvedKeychain, &auth.ResolvedKeychain{
					AuthConfigs: map[string]*authn.AuthConfig{},
				})
			})
		})

		when("passed no images", func() {
			it("returns an empty keychain", func() {
				keychain := &FakeKeychain{
					authMap: map[string]*authn.AuthConfig{
						"some-registry.com": {
							Username: "user",
							Password: "password",
						},
					},
				}

				resolvedKeychain := auth.NewResolvedKeychain(keychain)

				h.AssertEq(t, resolvedKeychain, &auth.ResolvedKeychain{
					AuthConfigs: map[string]*authn.AuthConfig{},
				})
			})
		})
	})

	when("ResolvedKeychain", func() {
		when("#Resolve", func() {
			var resolvedKeychain auth.ResolvedKeychain

			it.Before(func() {
				resolvedKeychain = auth.ResolvedKeychain{
					AuthConfigs: map[string]*authn.AuthConfig{
						"some-registry.com": {
							Username: "user",
							Password: "password",
						},
						"basic-registry.com": {
							Auth: "some-basic-auth=",
						},
						"bearer-registry.com": {
							RegistryToken: "some-bearer-auth=",
						},
						"oauth.registry.io": {
							IdentityToken: "some-identity-token=",
						},
					},
				}
			})

			when("auth config is found", func() {
				it("loads the basic auth from memory", func() {
					registry, err := name.NewRegistry("basic-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := resolvedKeychain.Resolve(registry)
					h.AssertNil(t, err)

					header, err := authenticator.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, header, &authn.AuthConfig{Auth: "some-basic-auth="})
				})

				it("loads the bearer auth from memory", func() {
					registry, err := name.NewRegistry("bearer-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := resolvedKeychain.Resolve(registry)
					h.AssertNil(t, err)

					header, err := authenticator.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, header, &authn.AuthConfig{RegistryToken: "some-bearer-auth="})
				})

				it("loads the identity token from memory", func() {
					registry, err := name.NewRegistry("oauth.registry.io", name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := resolvedKeychain.Resolve(registry)
					h.AssertNil(t, err)

					header, err := authenticator.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, header, &authn.AuthConfig{IdentityToken: "some-identity-token="})
				})
			})

			when("auth config is not found", func() {
				it("returns an Anonymous authenticator", func() {
					registry, err := name.NewRegistry("no-auth-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := resolvedKeychain.Resolve(registry)
					h.AssertNil(t, err)

					_, err = authenticator.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, authenticator, authn.Anonymous)
				})
			})

			when("keychain is empty", func() {
				it.Before(func() {
					resolvedKeychain = auth.ResolvedKeychain{AuthConfigs: map[string]*authn.AuthConfig{}}
				})

				it("returns an Anonymous authenticator", func() {
					registry, err := name.NewRegistry("some-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := resolvedKeychain.Resolve(registry)
					h.AssertNil(t, err)

					_, err = authenticator.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, authenticator, authn.Anonymous)
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
					"empty-auth.registry.io": {},
					"oauth.registry.io": {
						IdentityToken: "hjkl=",
					},
				},
			}
		})

		it("builds json encoded env with auth headers", func() {
			envVar, err := auth.BuildEnvVar(keychain,
				"some-registry.com/image1",
				"some-registry.com/image2",
				"other-registry.com/image",
				"my/image", // index.docker.io
				"oauth.registry.io/image",
			)
			h.AssertNil(t, err)

			var jsonAuth bytes.Buffer
			h.AssertNil(t, json.Compact(&jsonAuth, []byte(`{
	"index.docker.io": "Bearer qwerty=",
	"oauth.registry.io": "X-Identity hjkl=",
	"other-registry.com": "Basic asdf=",
	"some-registry.com": "Basic dXNlcjpwYXNzd29yZA=="
}`)))
			h.AssertEq(t, envVar, jsonAuth.String())
		})

		when("anonymous registry", func() {
			it("returns an empty result", func() {
				envVar, err := auth.BuildEnvVar(keychain, "anonymous.com/some/image")
				h.AssertNil(t, err)

				h.AssertEq(t, envVar, "{}")
			})
		})

		when("auth config is empty", func() {
			it("returns an empty result", func() {
				envVar, err := auth.BuildEnvVar(keychain, "empty-auth.registry.io/some/image")
				h.AssertNil(t, err)

				h.AssertEq(t, envVar, "{}")
			})
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
