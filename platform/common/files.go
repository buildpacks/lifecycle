package common

import (
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/pkg/errors"
)

type StackMetadata struct {
	RunImage StackRunImageMetadata `json:"runImage" toml:"run-image"`
}

type StackRunImageMetadata struct {
	Image   string   `toml:"image" json:"image"`
	Mirrors []string `toml:"mirrors" json:"mirrors,omitempty"`
}

func (sm *StackMetadata) BestRunImageMirror(registry string) (string, error) {
	if sm.RunImage.Image == "" {
		return "", errors.New("missing run-image metadata")
	}
	runImageMirrors := []string{sm.RunImage.Image}
	runImageMirrors = append(runImageMirrors, sm.RunImage.Mirrors...)
	runImageRef, err := byRegistry(registry, runImageMirrors)
	if err != nil {
		return "", errors.Wrap(err, "failed to find run-image")
	}
	return runImageRef, nil
}

func byRegistry(reg string, imgs []string) (string, error) {
	if len(imgs) < 1 {
		return "", errors.New("no images provided to search")
	}

	for _, img := range imgs {
		ref, err := name.ParseReference(img, name.WeakValidation)
		if err != nil {
			continue
		}
		if reg == ref.Context().RegistryStr() {
			return img, nil
		}
	}
	return imgs[0], nil
}
