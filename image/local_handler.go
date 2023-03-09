package image

import (
	"errors"

	"github.com/buildpacks/imgutil"
	"github.com/buildpacks/imgutil/local"
	"github.com/docker/docker/client"
)

const LocalKind = "docker"

type LocalHandler struct {
	docker client.CommonAPIClient
}

func NewLocalHandler(opts HandlerOptions) (*LocalHandler, error) {
	if opts.DockerClient == nil {
		// we only ever have one docker client, so it must be provided when instantiating the handler
		return nil, errors.New("docker client must be provided when exporting to daemon")
	}
	return &LocalHandler{docker: opts.DockerClient}, nil
}

func (h *LocalHandler) CheckReadAccess(imageRef string) (bool, error) {
	// TODO: verify that we can find the image in the daemon
	return true, nil
}

func (h *LocalHandler) InitImage(imageRef string) (imgutil.Image, error) {
	if imageRef == "" {
		return nil, nil
	}

	return local.NewImage(
		imageRef,
		h.docker,
		local.FromBaseImage(imageRef),
	)
}

func (h *LocalHandler) Kind() string {
	return LocalKind
}
