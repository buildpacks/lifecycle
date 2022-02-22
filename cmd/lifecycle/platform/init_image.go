package platform

import (
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/buildpacks/imgutil/remote"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
)

type imageHandler struct {
	docker   client.CommonAPIClient
	keychain authn.Keychain
}

func (h *imageHandler) initImage(imageRef string) (imgutil.Image, error) {
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
