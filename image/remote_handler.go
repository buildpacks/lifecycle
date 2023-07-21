package image

import (
	"strings"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/remote"
	"github.com/google/go-containerregistry/pkg/authn"
)

const RemoteKind = "remote"

type RemoteHandler struct {
	keychain         authn.Keychain
	insecureRegistry string
}

func (h *RemoteHandler) InitImage(imageRef string) (imgutil.Image, error) {
	if imageRef == "" {
		return nil, nil
	}

	options := []remote.ImageOption{
		remote.FromBaseImage(imageRef),
	}

	if h.insecureRegistry != "" && strings.HasPrefix(imageRef, h.insecureRegistry) {
		options = append(options, remote.WithRegistrySetting(h.insecureRegistry, true, true))
	}

	return remote.NewImage(
		imageRef,
		h.keychain,
		options...,
	)
}

func (h *RemoteHandler) Kind() string {
	return RemoteKind
}
