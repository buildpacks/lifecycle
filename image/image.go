package image

import (
	"fmt"

	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
)

type RegistryInputs interface {
	ReadableImages() []string
	WriteableImages() []string
}

// ValidateDestinationTags ensures all tags are valid
// daemon - when false (exporting to a registry), ensures all tags are on the same registry
func ValidateDestinationTags(daemon bool, repoNames ...string) error {
	var (
		reg        string
		registries = map[string]struct{}{}
	)

	for _, repoName := range repoNames {
		ref, err := name.ParseReference(repoName, name.WeakValidation)
		if err != nil {
			return err
		}
		reg = ref.Context().RegistryStr()
		registries[reg] = struct{}{}
	}

	if !daemon && len(registries) != 1 {
		return errors.New("writing to multiple registries is unsupported")
	}

	return nil
}

func VerifyRegistryAccess(regInputs RegistryInputs, keychain authn.Keychain) error {
	if len(regInputs.ReadableImages()) > 0 {
		for _, imageRef := range regInputs.ReadableImages() {
			err := verifyReadAccess(imageRef, keychain)
			if err != nil {
				return err
			}
		}
	}

	if len(regInputs.WriteableImages()) > 0 {
		for _, imageRef := range regInputs.WriteableImages() {
			err := verifyReadWriteAccess(imageRef, keychain)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyReadAccess(imageRef string, keychain authn.Keychain) error {
	if imageRef != "" {
		img, _ := remote.NewImage(imageRef, keychain)
		if !img.CheckReadAccess() {
			return errors.New(fmt.Sprintf("read image %s from the registry", imageRef))
		}
	}
	return nil
}

func verifyReadWriteAccess(imageRef string, keychain authn.Keychain) error {
	if imageRef != "" {
		img, _ := remote.NewImage(imageRef, keychain)
		if !img.CheckReadWriteAccess() {
			return errors.New(fmt.Sprintf("read/write image %s from/to the registry", imageRef))
		}
	}
	return nil
}
