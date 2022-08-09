package kaniko

import (
	"fmt"
	"os"
	"time"

	"github.com/GoogleContainerTools/kaniko/pkg/config"
	"github.com/GoogleContainerTools/kaniko/pkg/executor"
	"github.com/containerd/containerd/platforms"

	"github.com/buildpacks/lifecycle/internal/dockerfile"
	"github.com/buildpacks/lifecycle/log"
)

type DockerfileApplier struct {
	cacheImageRef          string
	dockerfileBuildContext string
}

func NewDockerfileApplier() *DockerfileApplier {
	return &DockerfileApplier{
		cacheImageRef:          "oci:/kaniko/cache-dir/cache-image", // TODO
		dockerfileBuildContext: "",                                  // TODO
	}
}

func (a *DockerfileApplier) Apply(dockerfiles []dockerfile.Dockerfile, fromImageRef string, logger log.Logger) error {
	for idx, dfile := range dockerfiles {
		opts := config.KanikoOptions{
			BuildArgs:         append(toMultiArg(dfile.Args), fmt.Sprintf(`base_image=%s`, fromImageRef)),
			Cache:             true,
			CacheOptions:      config.CacheOptions{CacheDir: "/workspace/cache", CacheTTL: 14 * (24 * time.Hour)}, // TODO: make configurable
			CacheRepo:         a.cacheImageRef,                                                                    // an oci-layout image
			Cleanup:           false,
			CustomPlatform:    platforms.DefaultString(),
			DockerfilePath:    dfile.Path,
			IgnoreVarRun:      true, // TODO: make configurable, add ignore paths
			NoPush:            true,
			Reproducible:      true,
			SkipInitialUnpack: true,
			SnapshotMode:      "full",
			SrcContext:        a.dockerfileBuildContext, // TODO: figure out what this directory should be and add it - /layers???
		}

		// kaniko does this here: https://github.com/GoogleContainerTools/kaniko/blob/09e70e44d9e9a3fecfcf70cb809a654445837631/cmd/executor/cmd/root.go#L140-L142
		if err := os.Chdir("/"); err != nil {
			return err
		}

		logger.Debugf("Applying the Dockerfile at %s...", dfile.Path)
		intermediateImage, err := executor.DoBuild(&opts)
		if err != nil {
			return fmt.Errorf("applying dockerfile: %s", err.Error())
		}
		intermediateImageHash, err := intermediateImage.Digest()
		if err != nil {
			return fmt.Errorf("getting hash for intermediate image %d: %s", idx+1, err.Error())
		}
		fromImageRef = fromImageRef + "-extended@sha256:" + intermediateImageHash.String()
	}

	return nil
}

func toMultiArg(args []dockerfile.Arg) []string {
	var result []string
	for _, arg := range args {
		result = append(result, fmt.Sprintf("%s=%s", arg.Name, arg.Value))
	}
	return result
}
