package auth_test

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/auth"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestEnvKeychain(t *testing.T) {
	spec.Run(t, "NewKeychain", testEnvKeychain, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testEnvKeychain(t *testing.T, when spec.G, it spec.S) {
	when("#NewKeychain", func() {
		when("CNB_REGISTRY_AUTH is set", func() {
			it.Before(func() {
				err := os.Setenv(
					"CNB_REGISTRY_AUTH",
					`foo`,
				)
				h.AssertNil(t, err)
			})

			it.After(func() {
				err := os.Unsetenv("CNB_REGISTRY_AUTH")
				h.AssertNil(t, err)
			})

			it("returns an EnvKeychain", func() {
				envKeyChain := auth.NewKeychain("CNB_REGISTRY_AUTH")
				_, ok := envKeyChain.(*auth.EnvKeychain)
				if ok != true {
					t.Fatalf("expected *auth.EnvKeychain, got %s", reflect.TypeOf(envKeyChain))
				}
			})
		})

		when("CNB_REGISTRY_AUTH is not set", func() {
			it("returns the ggcr DefaultKeychain", func() {
				envKeyChain := auth.NewKeychain("CNB_REGISTRY_AUTH")
				h.AssertEq(t, envKeyChain, authn.DefaultKeychain)
			})
		})
	})

	when("#EnvKeychain", func() {
		var envKeyChain authn.Keychain

		when("#Resolve", func() {
			it.Before(func() {
				envKeyChain = &auth.EnvKeychain{EnvVar: "CNB_REGISTRY_AUTH"}
			})

			it.After(func() {
				err := os.Unsetenv("CNB_REGISTRY_AUTH")
				h.AssertNil(t, err)
			})

			when("valid auth env variable is set", func() {
				it.Before(func() {
					err := os.Setenv(
						"CNB_REGISTRY_AUTH",
						`{"basic-registry.com": "Basic asdf=", "bearer-registry.com": "Bearer qwerty="}`,
					)
					h.AssertNil(t, err)
				})

				it("loads the basic auth from the environment", func() {
					registry, err := name.NewRegistry("basic-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := envKeyChain.Resolve(registry)
					h.AssertNil(t, err)

					header, err := authenticator.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, header, &authn.AuthConfig{Auth: "asdf="})
				})

				it("loads the bearer auth from the environment", func() {
					registry, err := name.NewRegistry("bearer-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := envKeyChain.Resolve(registry)
					h.AssertNil(t, err)

					header, err := authenticator.Authorization()
					h.AssertNil(t, err)

					h.AssertEq(t, header, &authn.AuthConfig{RegistryToken: "qwerty="})
				})

				it("returns an Anonymous authenticator when the environment does not have a auth header", func() {
					envKeyChain = auth.NewKeychain("CNB_REGISTRY_AUTH")
					registry, err := name.NewRegistry("no-env-auth-registry.com", name.WeakValidation)
					h.AssertNil(t, err)

					authenticator, err := envKeyChain.Resolve(registry)
					h.AssertNil(t, err)

					h.AssertEq(t, authenticator, authn.Anonymous)
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
	authMap map[string]*authn.AuthConfig
}

func (f *FakeKeychain) Resolve(r authn.Resource) (authn.Authenticator, error) {
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
