package image

import (
	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
)

const RemoteKind = "remote"

type RemoteHandler struct {
	keychain           authn.Keychain
	insecureRegistries []string
}

func (h *RemoteHandler) InitImage(imageRef string) (imgutil.Image, error) {
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

// InitRemoteImage TODO
func (h *RemoteHandler) InitRemoteImage(imageRef string) (imgutil.Image, error) {
	return h.InitImage(imageRef)
}

func (h *RemoteHandler) Kind() string {
	return RemoteKind
}
