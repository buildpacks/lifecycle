package files

import (
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/lifecycle/log"
)

// Stack (deprecated as of Platform API 0.12) is provided by the platform as stack.toml to record information about the run images
// that may be used during export.
// It is also serialized by the exporter as the `stack` key in the `io.buildpacks.lifecycle.metadata` label on the output image
// for use during rebase.
type Stack struct {
	RunImage RunImageForExport `json:"runImage" toml:"run-image"`
}

type RunImageForExport struct {
	Image   string   `toml:"image" json:"image"`
	Mirrors []string `toml:"mirrors" json:"mirrors,omitempty"`
}

func ReadStack(stackPath string, logger log.Logger) (Stack, error) {
	var stackMD Stack
	if _, err := toml.DecodeFile(stackPath, &stackMD); err != nil {
		if os.IsNotExist(err) {
			logger.Infof("no stack metadata found at path '%s'\n", stackPath)
			return Stack{}, nil
		}
		return Stack{}, err
	}
	return stackMD, nil
}

// FIXME: the mirror logic in this file might be better located in a dedicated package.

func (sm *Stack) BestRunImageMirrorFor(registry string) (string, error) {
	return sm.RunImage.BestRunImageMirrorFor(registry)
}

func (rm *RunImageForExport) BestRunImageMirrorFor(registry string) (string, error) {
	if rm.Image == "" {
		return "", errors.New("missing run-image metadata")
	}
	runImageMirrors := []string{rm.Image}
	runImageMirrors = append(runImageMirrors, rm.Mirrors...)
	runImageRef, err := byRegistry(registry, runImageMirrors)
	if err != nil {
		return "", fmt.Errorf("failed to find run image: %w", err)
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
