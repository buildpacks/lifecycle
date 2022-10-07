//go:build windows

package kaniko

import (
	"github.com/buildpacks/lifecycle/internal/extend"
)

func (a *DockerfileApplier) Apply(workspace string, baseImageRef string, dockerfiles []extend.Dockerfile, options extend.Options) error {
	return nil
}
