//go:build linux

package kaniko

import (
	"fmt"
	"os"

	"github.com/GoogleContainerTools/kaniko/pkg/config"
	"github.com/GoogleContainerTools/kaniko/pkg/executor"
	"github.com/GoogleContainerTools/kaniko/pkg/image"
	v1 "github.com/google/go-containerregistry/pkg/v1"

	"github.com/buildpacks/lifecycle/internal/extend"
	"github.com/buildpacks/lifecycle/log"
)

func (a *DockerfileApplier) Apply(dockerfile extend.Dockerfile, toBaseImage v1.Image, withBuildOptions extend.Options, logger log.Logger) (v1.Image, error) {
	// configure kaniko
	image.RetrieveRemoteImage = func(image string, opts config.RegistryOptions, customPlatform string) (v1.Image, error) {
		return toBaseImage, nil // force kaniko to return the provided base image, instead of trying to pull it from a registry
	}

	// get digest ref for options
	digestToExtend, err := toBaseImage.Digest()
	if err != nil {
		return nil, fmt.Errorf("failed to get digest: %w", err)
	}
	baseImageRef := fmt.Sprintf("base@%s", digestToExtend)
	opts := createOptions(baseImageRef, dockerfile, withBuildOptions)

	// change to root directory; kaniko does this here:
	// https://github.com/GoogleContainerTools/kaniko/blob/09e70e44d9e9a3fecfcf70cb809a654445837631/cmd/executor/cmd/root.go#L140-L142
	if err = os.Chdir("/"); err != nil {
		return nil, err
	}

	// apply Dockerfile
	logger.Debugf("Applying Dockerfile at %s to '%s'...", dockerfile.Path, baseImageRef)
	return executor.DoBuild(&opts)
}
