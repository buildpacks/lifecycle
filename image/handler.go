package image

import (
	"github.com/buildpacks/imgutil"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
)

type Handler interface {
	InitImage(imageRef string) (imgutil.Image, error)
	Docker() bool
	Layout() bool
}

func NewHandler(docker client.CommonAPIClient, keychain authn.Keychain, layoutDir string, useLayout bool) Handler {
	if layoutDir != "" && useLayout {
		return &LayoutHandler{
			layoutDir: layoutDir,
		}
	}
	if docker != nil {
		return &LocalHandler{
			docker: docker,
		}
	}
	if keychain != nil {
		return &RemoteHandler{
			keychain: keychain,
		}
	}
	return nil
}
