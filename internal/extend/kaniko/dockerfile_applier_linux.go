//go:build linux

package kaniko

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GoogleContainerTools/kaniko/pkg/config"
	"github.com/GoogleContainerTools/kaniko/pkg/executor"
	"github.com/GoogleContainerTools/kaniko/pkg/image"
	"github.com/containerd/containerd/platforms"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"

	"github.com/buildpacks/lifecycle/internal/extend"
	"github.com/buildpacks/lifecycle/log"
)

func (a *DockerfileApplier) Apply(dockerfiles []extend.Dockerfile, baseImageRef string, logger log.Logger) error {
	// Configure kaniko
	executor.InitialFSUnpacked = true // the extender is running in the context of the base image to extend, so there is no need to unpack an FS
	baseImage, err := readOCI(baseImageRef)
	if err != nil {
		return fmt.Errorf("getting base image from reference '%s': %w", baseImageRef, err)
	}
	image.RetrieveRemoteImage = func(image string, opts config.RegistryOptions, customPlatform string) (v1.Image, error) {
		return baseImage, nil // force kaniko to return this base image, instead of trying to pull it from a registry
	}

	// Range over Dockerfiles
	for idx, dfile := range dockerfiles {
		opts := config.KanikoOptions{
			BuildArgs:      append(toList(dfile.Args), fmt.Sprintf(`base_image=%s`, baseImageRef)),
			Cache:          true,
			CacheOptions:   config.CacheOptions{CacheDir: a.cacheDir, CacheTTL: 14 * (24 * time.Hour)}, // TODO (before merging): make TTL configurable
			CacheRepo:      a.cacheImageRef,
			Cleanup:        false,
			CustomPlatform: platforms.DefaultString(),
			DockerfilePath: dfile.Path,
			IgnorePaths:    []string{"/layers", "/platform", "/workspace"}, // TODO (before merging): make configurable and test
			IgnoreVarRun:   true,
			NoPush:         true,
			Reproducible:   false, // If Reproducible=true kaniko will try to read the base image layers, requiring the lifecycle to pull them
			SnapshotMode:   "full",
			SrcContext:     a.dockerfileBuildContext,
		}

		// Change to root directory; kaniko does this here:
		// https://github.com/GoogleContainerTools/kaniko/blob/09e70e44d9e9a3fecfcf70cb809a654445837631/cmd/executor/cmd/root.go#L140-L142
		if err := os.Chdir("/"); err != nil {
			return err
		}

		// Apply Dockerfile
		var err error
		logger.Debugf("Applying the Dockerfile at %s...", dfile.Path)
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
	if !strings.HasPrefix(digestRef, "oci:") { // TODO (before merging): parse reference properly with GGCR
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
	v1Image, err := path.Image(hash) // TODO (before merging): use selective.Image
	if err != nil {
		return nil, fmt.Errorf("getting image from hash '%s': %w", hash.String(), err)
	}
	return v1Image, nil
}

func toList(args []extend.Arg) []string {
	var result []string
	for _, arg := range args {
		result = append(result, fmt.Sprintf("%s=%s", arg.Name, arg.Value))
	}
	return result
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
