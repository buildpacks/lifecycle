package image

import (
	"strings"

	"github.com/buildpacks/lifecycle/cmd"

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

	options := []remote.ImageOption{
		remote.FromBaseImage(imageRef),
	}

	options = append(options, h.getInsecureRegistryOptions(imageRef)...)

	return remote.NewImage(
		imageRef,
		h.keychain,
		options...,
	)
}

func (h *RemoteHandler) Kind() string {
	return RemoteKind
}
func (h *RemoteHandler) getInsecureRegistryOptions(imageRef string) []remote.ImageOption {
	var opts []remote.ImageOption
	if len(h.insecureRegistries) > 0 {
		cmd.DefaultLogger.Warnf("Found Insecure Registries: %+q", h.insecureRegistries)
		for _, insecureRegistry := range h.insecureRegistries {
			if strings.HasPrefix(imageRef, insecureRegistry) {
				opts = append(opts, remote.WithRegistrySetting(insecureRegistry, true, true))
			}
		}
	}
	return opts
}
