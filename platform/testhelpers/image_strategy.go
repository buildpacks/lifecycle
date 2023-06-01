package testhelpers

import (
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/buildpacks/lifecycle/platform"
)

type TestResource struct {
	Repo string
}

var _ authn.Resource = &TestResource{}

func (t *TestResource) String() string {
	return t.Repo
}

func (t *TestResource) RegistryStr() string {
	return t.Repo
}

type SimpleImageStrategy struct{}

var _ platform.ImageStrategy = &SimpleImageStrategy{}

func (t *SimpleImageStrategy) CheckReadAccess(repo string, keychain authn.Keychain) (bool, error) {
	resource := &TestResource{Repo: repo}
	_, err := keychain.Resolve(resource)

	return (err == nil), err
}

type StubImageStrategy struct {
	AllowedRepo string
}

var _ platform.ImageStrategy = &StubImageStrategy{}

func (s *StubImageStrategy) CheckReadAccess(repo string, _ authn.Keychain) (bool, error) {
	return strings.Contains(repo, s.AllowedRepo), nil
}
