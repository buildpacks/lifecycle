package inputs

import (
	"fmt"

	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
)

//go:generate mockgen -package testmock -destination testmock/registry_handler.go github.com/buildpacks/lifecycle/platform/inputs RegistryHandler
type RegistryHandler interface {
	EnsureReadAccess(imageRefs []string) error
	EnsureWriteAccess(imageRefs []string) error
}

type DefaultRegistryHandler struct {
	keychain authn.Keychain
}

func NewRegistryHandler(keychain authn.Keychain) *DefaultRegistryHandler {
	return &DefaultRegistryHandler{
		keychain: keychain,
	}
}

func (rv *DefaultRegistryHandler) EnsureReadAccess(imageRefs []string) error {
	for _, imageRef := range imageRefs {
		if err := verifyReadAccess(imageRef, rv.keychain); err != nil {
			return err
		}
	}
	return nil
}

func (rv *DefaultRegistryHandler) EnsureWriteAccess(imageRefs []string) error {
	for _, imageRef := range imageRefs {
		if err := verifyReadWriteAccess(imageRef, rv.keychain); err != nil {
			return err
		}
	}
	return nil
}

func verifyReadAccess(imageRef string, keychain authn.Keychain) error {
	img, _ := remote.NewImage(imageRef, keychain)
	if !img.CheckReadAccess() {
		return errors.Errorf("ensure registry read access to %s", imageRef)
	}
	return nil
}

func verifyReadWriteAccess(imageRef string, keychain authn.Keychain) error {
	img, _ := remote.NewImage(imageRef, keychain)
	if !img.CheckReadWriteAccess() {
		return errors.Errorf("ensure registry read/write access to %s", imageRef)
	}
	return nil
}

// registry helpers

func appendNotEmpty(slice []string, elems ...string) []string {
	for _, v := range elems {
		if v != "" {
			slice = append(slice, v)
		}
	}
	return slice
}

func ensureSameRegistry(firstRef string, secondRef string) error {
	if firstRef == secondRef {
		return nil
	}
	firstRegistry, err := parseRegistry(firstRef)
	if err != nil {
		return err
	}
	secondRegistry, err := parseRegistry(secondRef)
	if err != nil {
		return err
	}
	if firstRegistry != secondRegistry {
		return fmt.Errorf("writing to multiple registries is unsupported: %s, %s", firstRegistry, secondRegistry)
	}
	return nil
}

func parseRegistry(providedRef string) (string, error) {
	ref, err := name.ParseReference(providedRef, name.WeakValidation)
	if err != nil {
		return "", err
	}
	return ref.Context().RegistryStr(), nil
}
