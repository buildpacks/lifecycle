package kaniko

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GoogleContainerTools/kaniko/pkg/config"
	"github.com/containerd/containerd/platforms"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/layout"

	"github.com/buildpacks/lifecycle/internal/extend"
)

const (
	kanikoDir = "/kaniko"
	ociPrefix = "oci:"
)

var (
	kanikoBaseCacheDir  = filepath.Join(kanikoDir, "cache", "base")
	kanikoCacheImageRef = filepath.Join(ociPrefix, kanikoDir, "cache", "layers", "cached")
)

type DockerfileApplier struct{}

func (a *DockerfileApplier) ImageFor(reference string) (v1.Image, error) {
	digest, err := name.NewDigest(reference)
	if err != nil {
		return nil, fmt.Errorf("failed to get digest for reference '%s': %w", reference, err)
	}
	baseImage, err := readOCI(ociPrefix + filepath.Join(kanikoDir, "cache", "base", digest.DigestStr()))
	if err != nil {
		return nil, fmt.Errorf("getting base image for digest '%s': %w", digest, err)
	}
	return baseImage, nil
}

func readOCI(path string) (v1.Image, error) {
	if !strings.HasPrefix(path, "oci:") {
		return nil, fmt.Errorf("expected '%s' to have prefix 'oci:'", path)
	}
	layoutPath, err := layout.FromPath(strings.TrimPrefix(path, "oci:"))
	if err != nil {
		return nil, fmt.Errorf("getting layout from path: %w", err)
	}
	hash, err := v1.NewHash(filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("getting hash from reference '%s': %w", path, err)
	}
	v1Image, err := layoutPath.Image(hash) // FIXME: we may want to implement path.Image(h) in the imgutil 'sparse' package so that trying to access layers on this image errors with a helpful message
	if err != nil {
		return nil, fmt.Errorf("getting image from hash '%s': %w", hash.String(), err)
	}
	return v1Image, nil
}

func (a *DockerfileApplier) Cleanup() error {
	fis, err := os.ReadDir(kanikoDir)
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

func createOptions(baseImageRef string, dockerfile extend.Dockerfile, options extend.Options) config.KanikoOptions {
	return config.KanikoOptions{
		BuildArgs:         append(toArgList(dockerfile.Args), fmt.Sprintf(`base_image=%s`, baseImageRef)),
		Cache:             true,
		CacheOptions:      config.CacheOptions{CacheDir: kanikoBaseCacheDir, CacheTTL: options.CacheTTL},
		CacheRunLayers:    true,
		CacheRepo:         kanikoCacheImageRef,
		Cleanup:           false,
		CustomPlatform:    platforms.DefaultString(),
		DockerfilePath:    dockerfile.Path,
		IgnorePaths:       options.IgnorePaths,
		IgnoreVarRun:      true,
		InitialFSUnpacked: true, // The executor is running in the context of the image being extended, so there is no need to unpack the filesystem
		NoPush:            true,
		Reproducible:      false, // If Reproducible=true kaniko will try to read the base image layers, requiring the lifecycle to pull them
		SnapshotMode:      "full",
		SrcContext:        options.BuildContext,
	}
}

func toArgList(args []extend.Arg) []string {
	var result []string
	for _, arg := range args {
		result = append(result, fmt.Sprintf("%s=%s", arg.Name, arg.Value))
	}
	return result
}
