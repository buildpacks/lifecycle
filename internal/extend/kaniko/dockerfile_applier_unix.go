//go:build darwin || freebsd

package kaniko

import (
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/buildpacks/lifecycle/internal/extend"
	"github.com/buildpacks/lifecycle/log"
)

func (a *DockerfileApplier) Apply(dockerfile extend.Dockerfile, toBaseImage v1.Image, withBuildOptions extend.Options, logger log.Logger) (v1.Image, error) {
	return nil, nil
}
