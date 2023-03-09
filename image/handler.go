package image

import (
	"github.com/buildpacks/imgutil"
	"github.com/docker/docker/client"
	"github.com/google/go-containerregistry/pkg/authn"
)

type Handler interface {
	CheckReadAccess(imageRef string) (bool, error)
	InitImage(imageRef string) (imgutil.Image, error)
	Kind() string
}

type HandlerOptions struct {
	DockerClient     client.CommonAPIClient
	RegistryKeychain authn.Keychain
	LayoutDir        string
	UseLayout        bool
	UseDaemon        bool
}

func NewHandler(opts HandlerOptions) (Handler, error) {
	if opts.UseLayout {
		return NewLayoutHandler(opts)
	}
	if opts.UseDaemon {
		return NewLocalHandler(opts)
	}
	return NewRemoteHandler(opts)
}

type NopHandler struct{}

func (h *NopHandler) CheckReadAccess(imageRef string) (bool, error) {
	return true, nil
}

func (h *NopHandler) InitImage(imageRef string) (imgutil.Image, error) {
	return nil, nil
}

func (h *NopHandler) Kind() string {
	return ""
}
