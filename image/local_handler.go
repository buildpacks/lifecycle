package image

import (
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
)

const LocalKind = "docker"

type LocalHandler struct {
	docker             client.CommonAPIClient
	keychain           authn.Keychain
	insecureRegistries []string
}

func (h *LocalHandler) InitImage(imageRef string) (imgutil.Image, error) {
	if imageRef == "" {
		return nil, nil
	}

	return local.NewImage(
		imageRef,
		h.docker,
		local.FromBaseImage(imageRef),
	)
}

// InitRemoteImage TODO
func (h *LocalHandler) InitRemoteImage(imageRef string) (imgutil.Image, error) {
	if imageRef == "" {
		return nil, nil
	}

	options := []imgutil.ImageOption{
		remote.FromBaseImage(imageRef),
	}

	options = append(options, GetInsecureOptions(h.insecureRegistries)...)

	return remote.NewImage(
		imageRef,
		h.keychain,
		options...,
	)
}

func (h *LocalHandler) Kind() string {
	return LocalKind
}
