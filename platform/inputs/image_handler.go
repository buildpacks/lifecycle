package inputs

import (
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
)

//go:generate mockgen -package testmock -destination testmock/image_handler.go github.com/buildpacks/lifecycle/platform/inputs ImageHandler
type ImageHandler interface {
	InitImage(imageRef string) (imgutil.Image, error)
	Docker() bool
}

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

func (h *DefaultImageHandler) Docker() bool {
	return h.docker != nil
}
