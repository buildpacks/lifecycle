//go:build darwin

package kaniko

import (
	"github.com/buildpacks/lifecycle/internal/extend"
	"github.com/buildpacks/lifecycle/log"
)

func (a *DockerfileApplier) Apply(dockerfiles []extend.Dockerfile, baseImageRef string, logger log.Logger) error {
	return nil
}
