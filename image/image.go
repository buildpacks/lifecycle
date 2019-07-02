package image

import (
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
)

func ByRegistry(registry string, images []string) (string, error) {
	if len(images) < 1 {
		return "", errors.New("no images provided to search")
	}

	for _, img := range images {
		reg, err := ParseRegistry(img)
		if err != nil {
			continue
		}
		if registry == reg {
			return img, nil
		}
	}
	return images[0], nil
}

func ParseRegistry(imageName string) (string, error) {
	ref, err := name.ParseReference(imageName, name.WeakValidation)
	if err != nil {
		return "", err
	}
	return ref.Context().RegistryStr(), nil
}

func EnsureSingleRegistry(repoNames ...string) (string, error) {
	set := make(map[string]interface{})

	var (
		err      error
		registry string
	)

	for _, repoName := range repoNames {
		registry, err = ParseRegistry(repoName)
		if err != nil {
			return "", errors.Wrapf(err, "parsing registry from repo '%s'", repoName)
		}
		set[registry] = nil
	}

	if len(set) != 1 {
		return "", errors.New("exporting to multiple registries is unsupported")
	}

	return registry, nil
}
