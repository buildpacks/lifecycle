package image

import (
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
)

func EnsureSingleRegistry(repoNames ...string) error {
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

	if len(registries) != 1 {
		return errors.New("exporting to multiple registries is unsupported")
	}

	return nil
}
