package image

import (
	"errors"
	"fmt"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
)

const RemoteKind = "remote"

type RemoteHandler struct {
	keychain authn.Keychain
}

func NewRemoteHandler(opts HandlerOptions) (*RemoteHandler, error) {
	if opts.RegistryKeychain == nil {
		return nil, errors.New("keychain must be provided when exporting to registry")
	}
	return &RemoteHandler{keychain: opts.RegistryKeychain}, nil
}

func (h *RemoteHandler) CheckReadAccess(imageRef string) (bool, error) {
	img, err := remote.NewImage(imageRef, h.keychain)
	if err != nil {
		return false, fmt.Errorf("failed to get remote image: %w", err)
	}
	return img.CheckReadAccess(), nil
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

func (h *RemoteHandler) Kind() string {
	return RemoteKind
}
