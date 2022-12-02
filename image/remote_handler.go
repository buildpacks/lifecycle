package image

import (
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
)

type RemoteHandler struct {
	keychain authn.Keychain
}

func (h *RemoteHandler) InitImage(imageRef string) (imgutil.Image, error) {
	if imageRef == "" {
		return nil, nil
	}

	return remote.NewImage(
		imageRef,
		h.keychain,
		remote.FromBaseImage(imageRef),
	)
}

func (h *RemoteHandler) Docker() bool {
	return false
}

func (h *RemoteHandler) Layout() bool {
	return false
}
