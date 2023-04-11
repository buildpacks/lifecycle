//go:build linux

package kaniko

import (
	"fmt"
	"os"

	"github.com/GoogleContainerTools/kaniko/pkg/config"
	"github.com/GoogleContainerTools/kaniko/pkg/executor"
	"github.com/GoogleContainerTools/kaniko/pkg/image"
	"github.com/GoogleContainerTools/kaniko/pkg/util"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"

	"github.com/buildpacks/lifecycle/internal/extend"
	"github.com/buildpacks/lifecycle/log"
)

func (a *DockerfileApplier) Apply(dockerfile extend.Dockerfile, toBaseImage v1.Image, withBuildOptions extend.Options, logger log.Logger) (v1.Image, error) {
	if !inContainer() {
		return nil, errors.New("kaniko should only be run inside of a container")
	}

	// configure kaniko
	image.RetrieveRemoteImage = func(image string, opts config.RegistryOptions, customPlatform string) (v1.Image, error) {
		return toBaseImage, nil // force kaniko to return the provided base image, instead of trying to pull it from a registry
	}
	workDir, err := os.MkdirTemp(kanikoDir, "work.dir")
	if err != nil {
		return nil, err
	}
	config.KanikoDir = workDir

	// get digest ref for options
	digestToExtend, err := toBaseImage.Digest()
	if err != nil {
		return nil, fmt.Errorf("failed to get digest: %w", err)
	}
	baseImageRef := fmt.Sprintf("base@%s", digestToExtend)
	opts := createOptions(baseImageRef, dockerfile, withBuildOptions)

	// update ignore paths; kaniko does this here:
	// https://github.com/GoogleContainerTools/kaniko/blob/v1.9.2/cmd/executor/cmd/root.go#L124
	if opts.IgnoreVarRun {
		// from kaniko:
		// /var/run is a special case. It's common to mount in /var/run/docker.sock
		// or something similar which leads to a special mount on the /var/run/docker.sock
		// file itself, but the directory to exist in the image with no way to tell if it came
		// from the base image or not.
		util.AddToDefaultIgnoreList(util.IgnoreListEntry{
			Path:            "/var/run",
			PrefixMatchOnly: false,
		})
	}
	for _, p := range opts.IgnorePaths {
		util.AddToDefaultIgnoreList(util.IgnoreListEntry{
			Path:            p,
			PrefixMatchOnly: false,
		})
	}

	// change to root directory; kaniko does this here:
	// https://github.com/GoogleContainerTools/kaniko/blob/v1.9.2/cmd/executor/cmd/root.go#L160
	if err = os.Chdir("/"); err != nil {
		return nil, err
	}

	// apply Dockerfile
	logger.Debugf("Applying Dockerfile at %s to '%s'...", dockerfile.Path, baseImageRef)
	extendedImage, err := executor.DoBuild(&opts)
	if err != nil {
		return nil, err
	}

	// cleanup
	if err = os.RemoveAll(workDir); err != nil {
		return nil, err
	}
	return mutate.CreatedAt(extendedImage, v1.Time{})
}

func inContainer() bool {
	return proc.GetContainerRuntime(0, 0) != proc.RuntimeNotFound
}
