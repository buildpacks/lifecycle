package kaniko

import (
	"fmt"
	"path/filepath"

	"github.com/GoogleContainerTools/kaniko/pkg/config"
	"github.com/containerd/containerd/platforms"

	"github.com/buildpacks/lifecycle/internal/extend"
	"github.com/buildpacks/lifecycle/log"
)

const (
	kanikoDir = "/kaniko"
	ociPrefix = "oci:"
)

var (
	kanikoBaseCacheDir  = filepath.Join(kanikoDir, "cache", "base")
	kanikoCacheImageRef = filepath.Join(ociPrefix, kanikoDir, "cache", "layers", "cached")
)

type DockerfileApplier struct {
	outputDir string
	logger    log.Logger
}

func NewDockerfileApplier(outputDir string, logger log.Logger) *DockerfileApplier {
	return &DockerfileApplier{
		outputDir: outputDir,
		logger:    logger,
	}
}

func createOptions(dockerfileBuildContext string, baseImageRef string, dockerfile extend.Dockerfile, options extend.Options) config.KanikoOptions {
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
		SrcContext:        dockerfileBuildContext,
	}
}

func toArgList(args []extend.Arg) []string {
	var result []string
	for _, arg := range args {
		result = append(result, fmt.Sprintf("%s=%s", arg.Name, arg.Value))
	}
	return result
}
