package image

import (
	"errors"

	"github.com/google/go-containerregistry/pkg/name"
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
