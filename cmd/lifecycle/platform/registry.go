package platform

import (
	"fmt"

	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
)

// TODO: add tests for registry validator

//go:generate mockgen -package testmock -destination testmock/registry_validator.go github.com/buildpacks/lifecycle/cmd/lifecycle/platform RegistryValidator
type RegistryValidator interface {
	ValidateReadAccess(imageRefs []string) error
	ValidateWriteAccess(imageRefs []string) error
}

type DefaultRegistryValidator struct {
	keychain authn.Keychain
}

func NewRegistryValidator(keychain authn.Keychain) *DefaultRegistryValidator {
	return &DefaultRegistryValidator{
		keychain: keychain,
	}
}

func (rv *DefaultRegistryValidator) ValidateReadAccess(imageRefs []string) error {
	for _, imageRef := range imageRefs {
		if err := verifyReadAccess(imageRef, rv.keychain); err != nil {
			return err
		}
	}
	return nil
}

func (rv *DefaultRegistryValidator) ValidateWriteAccess(imageRefs []string) error {
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
		return fmt.Errorf("registries are different: %s, %s", firstRegistry, secondRegistry)
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
