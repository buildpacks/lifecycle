package platform

import (
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
)

type DefaultImageHandler struct {
	docker   client.CommonAPIClient
	keychain authn.Keychain
}

func NewImageHandler(docker client.CommonAPIClient, keychain authn.Keychain) *DefaultImageHandler {
	return &DefaultImageHandler{
		docker:   docker,
		keychain: keychain,
	}
}

func (h *DefaultImageHandler) InitImage(imageRef string) (imgutil.Image, error) {
	if imageRef == "" {
		return nil, nil
	}

	if h.docker != nil {
		return local.NewImage(
			imageRef,
			h.docker,
			local.FromBaseImage(imageRef),
		)
	}

	return remote.NewImage(
		imageRef,
		h.keychain,
		remote.FromBaseImage(imageRef),
	)
}

func (h *DefaultImageHandler) Docker() client.CommonAPIClient {
	return h.docker
}

func (h *DefaultImageHandler) Keychain() authn.Keychain {
	return h.keychain
}
