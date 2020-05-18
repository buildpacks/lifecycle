package image

import (
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
)

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
