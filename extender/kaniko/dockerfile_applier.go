package kaniko

import (
	"fmt"
	"os"
	"time"

	"github.com/GoogleContainerTools/kaniko/pkg/config"
	"github.com/GoogleContainerTools/kaniko/pkg/executor"
	"github.com/containerd/containerd/platforms"
	"github.com/google/go-containerregistry/pkg/name"

	"github.com/buildpacks/lifecycle/extender"
)

const (
	buildKind = "build"
	runKind   = "run"
)

type DockerfileApplier struct {
	cacheDir   string
	contextDir string
	workDir    string
}

func NewDockerfileApplier(cacheDir, contextDir, workDir string) *DockerfileApplier {
	return &DockerfileApplier{
		cacheDir:   cacheDir,
		contextDir: contextDir,
		workDir:    workDir,
	}
}

func (a *DockerfileApplier) ApplyBuild(dockerfiles []extender.Dockerfile, baseImageRef, targetImageRef string, ignorePaths []string, logger extender.Logger) error {
	fromImageRef := baseImageRef
	for idx, dockerfile := range dockerfiles {
		if dockerfile.Type != buildKind {
			logger.Infof("Skipping Dockerfile %s of wrong kind...", dockerfile.Path)
			continue
		}

		destRef, err := name.NewTag(targetImageRef, name.WeakValidation)
		if err != nil {
			return fmt.Errorf("failed to parse target image ref %s: %w", targetImageRef, err)
		}

		// default to registry/dest/cache
		cacheRef := fmt.Sprintf("%s/cache", destRef)

		opts := config.KanikoOptions{
			BuildArgs:       append(toMultiArg(dockerfile.Args), fmt.Sprintf(`base_image=%s`, fromImageRef)),
			Cleanup:         idx < len(dockerfiles)-1, // cleanup after all but the last dockerfile
			Destinations:    []string{targetImageRef},
			DockerfilePath:  dockerfile.Path,
			IgnoreVarRun:    true,                                        // TODO: add ignore paths
			RegistryOptions: config.RegistryOptions{SkipTLSVerify: true}, // TODO: remove eventually
			SnapshotMode:    "full",
			SrcContext:      a.workDir,
			CustomPlatform:  platforms.DefaultString(),

			Cache:     true,
			CacheRepo: cacheRef,
			CacheOptions: config.CacheOptions{
				CacheTTL: 14 * (24 * time.Hour),
			},
		}

		if err := doKaniko(dockerfile.Path, opts, logger); err != nil {
			return err
		}

		// The base image for the next Dockerfile should be the one we just built
		fromImageRef = targetImageRef // TODO: use digest instead
	}
	return nil
}

func (a *DockerfileApplier) ApplyRun(dockerfiles []extender.Dockerfile, baseImageRef string, targetImageRef string, ignorePaths []string, logger extender.Logger) error {
	fromImageRef := baseImageRef
	for _, dockerfile := range dockerfiles {
		if dockerfile.Type != runKind {
			logger.Infof("Skipping Dockerfile %s of wrong kind...", dockerfile.Path)
			continue
		}

		destRef, err := name.NewTag(targetImageRef, name.WeakValidation)
		if err != nil {
			return fmt.Errorf("failed to parse target image ref %s: %w", targetImageRef, err)
		}

		// default to registry/dest/cache
		cacheRef := fmt.Sprintf("%s/cache", destRef)

		opts := config.KanikoOptions{
			BuildArgs:       append(toMultiArg(dockerfile.Args), fmt.Sprintf(`base_image=%s`, fromImageRef)),
			Cleanup:         true,
			Destinations:    []string{targetImageRef},
			DockerfilePath:  dockerfile.Path,
			IgnoreVarRun:    true,                                        // TODO: add ignore paths
			RegistryOptions: config.RegistryOptions{SkipTLSVerify: true}, // TODO: remove eventually
			SnapshotMode:    "full",
			SrcContext:      a.workDir,
			CustomPlatform:  platforms.DefaultString(),

			Cache:     true,
			CacheRepo: cacheRef,
			CacheOptions: config.CacheOptions{
				CacheTTL: 14 * (24 * time.Hour),
			},
		}

		if err := doKaniko(dockerfile.Path, opts, logger); err != nil {
			return err
		}

		// The base image for the next Dockerfile should be the one we just built
		fromImageRef = targetImageRef // TODO: use digest instead
	}
	logger.Debug("Done")
	return nil
}

func doKaniko(path string, opts config.KanikoOptions, logger extender.Logger) error {
	// kaniko does this here: https://github.com/GoogleContainerTools/kaniko/blob/09e70e44d9e9a3fecfcf70cb809a654445837631/cmd/executor/cmd/root.go#L140-L142
	if err := os.Chdir("/"); err != nil {
		return err
	}

	logger.Debugf("Applying the Dockerfile at %s...", path)
	logger.Debugf("Options used: %+v", opts)
	newImage, err := executor.DoBuild(&opts)
	if err != nil {
		return err
	}

	logger.Debug("Pushing the image to its destination...")
	return executor.DoPush(newImage, &opts)
}

func toMultiArg(args []extender.DockerfileArg) []string {
	var result []string
	for _, arg := range args {
		result = append(result, fmt.Sprintf("%s=%s", arg.Key, arg.Value))
	}
	return result
}
