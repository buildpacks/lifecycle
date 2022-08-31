package kaniko

import (
	"fmt"
	"path/filepath"

	"github.com/GoogleContainerTools/kaniko/pkg/config"
	"github.com/containerd/containerd/platforms"

	"github.com/buildpacks/lifecycle/internal/extend"
	"github.com/buildpacks/lifecycle/log"
)

const kanikoDir = "/kaniko"

var (
	kanikoCacheDir      = filepath.Join(kanikoDir, "cache", "base")
	kanikoCacheImageRef = filepath.Join("oci:", kanikoDir, "cache", "layers", "cached")
)

type DockerfileApplier struct {
	logger log.Logger
}

func NewDockerfileApplier(logger log.Logger) *DockerfileApplier {
	return &DockerfileApplier{
		logger: logger,
	}
}

func createOptions(workspace string, baseImageRef string, dockerfile extend.Dockerfile, options extend.Options) config.KanikoOptions {
	return config.KanikoOptions{
		BuildArgs:      append(toList(dockerfile.Args), fmt.Sprintf(`base_image=%s`, baseImageRef)),
		Cache:          true,
		CacheOptions:   config.CacheOptions{CacheDir: kanikoCacheDir, CacheTTL: options.CacheTTL},
		CacheRepo:      kanikoCacheImageRef,
		Cleanup:        false,
		CustomPlatform: platforms.DefaultString(),
		DockerfilePath: dockerfile.Path,
		IgnorePaths:    options.IgnorePaths,
		IgnoreVarRun:   true,
		NoPush:         true,
		Reproducible:   false, // If Reproducible=true kaniko will try to read the base image layers, requiring the lifecycle to pull them
		SnapshotMode:   "full",
		SrcContext:     workspace,
	}
}

func toList(args []extend.Arg) []string {
	var result []string
	for _, arg := range args {
		result = append(result, fmt.Sprintf("%s=%s", arg.Name, arg.Value))
	}
	return result
}
