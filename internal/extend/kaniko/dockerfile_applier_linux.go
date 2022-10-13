//go:build linux

package kaniko

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/GoogleContainerTools/kaniko/pkg/config"
	"github.com/GoogleContainerTools/kaniko/pkg/executor"
	"github.com/GoogleContainerTools/kaniko/pkg/image"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"

	"github.com/buildpacks/lifecycle/internal/extend"
)

func (a *DockerfileApplier) Apply(workspace string, digest string, dockerfiles []extend.Dockerfile, options extend.Options) error {
	// Configure kaniko
	baseImageRef := ociPrefix + filepath.Join(kanikoCacheDir, digest)
	baseImage, err := readOCI(baseImageRef)
	if err != nil {
		return fmt.Errorf("getting base image for digest '%s': %w", digest, err)
	}
	image.RetrieveRemoteImage = func(image string, opts config.RegistryOptions, customPlatform string) (v1.Image, error) {
		return baseImage, nil // force kaniko to return this base image, instead of trying to pull it from a registry
	}

	// Range over Dockerfiles
	for idx, dfile := range dockerfiles {
		opts := createOptions(workspace, baseImageRef, dfile, options)

		// Change to root directory; kaniko does this here:
		// https://github.com/GoogleContainerTools/kaniko/blob/09e70e44d9e9a3fecfcf70cb809a654445837631/cmd/executor/cmd/root.go#L140-L142
		if err := os.Chdir("/"); err != nil {
			return err
		}

		// Apply Dockerfile
		var err error
		a.logger.Debugf("Applying the Dockerfile at %s...", dfile.Path)
		baseImage, err = executor.DoBuild(&opts)
		if err != nil {
			return fmt.Errorf("applying dockerfile: %w", err)
		}

		// Update base image to point to intermediate image
		intermediateImageHash, err := baseImage.Digest()
		if err != nil {
			return fmt.Errorf("getting hash for intermediate image %d: %w", idx+1, err)
		}
		baseImageRef = "intermediate-extended@" + intermediateImageHash.String()
	}

	// Set environment variables from the extended build image in the build context
	extendedConfig, err := baseImage.ConfigFile()
	if err != nil {
		return fmt.Errorf("getting config for extended image: %w", err)
	}
	for _, env := range extendedConfig.Config.Env {
		parts := strings.Split(env, "=")
		if len(parts) != 2 {
			return fmt.Errorf("parsing env '%s': expected format 'key=value'", env)
		}
		if err := os.Setenv(parts[0], parts[1]); err != nil {
			return fmt.Errorf("setting env: %w", err)
		}
	}

	return cleanKanikoDir()
}

func readOCI(digestRef string) (v1.Image, error) {
	if !strings.HasPrefix(digestRef, "oci:") {
		return nil, fmt.Errorf("expected '%s' to have prefix 'oci:'", digestRef)
	}
	path, err := layout.FromPath(strings.TrimPrefix(digestRef, "oci:"))
	if err != nil {
		return nil, fmt.Errorf("getting layout from path: %w", err)
	}
	hash, err := v1.NewHash(filepath.Base(digestRef))
	if err != nil {
		return nil, fmt.Errorf("getting hash from reference '%s': %w", digestRef, err)
	}
	v1Image, err := path.Image(hash) // // FIXME: we may want to implement path.Image(h) in the `selective` package so that trying to access layers on this image errors with a helpful message
	if err != nil {
		return nil, fmt.Errorf("getting image from hash '%s': %w", hash.String(), err)
	}
	return v1Image, nil
}

func cleanKanikoDir() error {
	fis, err := ioutil.ReadDir(kanikoDir)
	if err != nil {
		return fmt.Errorf("reading kaniko dir '%s': %w", kanikoDir, err)
	}
	for _, fi := range fis {
		if fi.Name() == "cache" {
			continue
		}
		toRemove := filepath.Join(kanikoDir, fi.Name())
		if err = os.RemoveAll(toRemove); err != nil {
			return fmt.Errorf("removing directory item '%s': %w", toRemove, err)
		}
	}
	return nil
}
