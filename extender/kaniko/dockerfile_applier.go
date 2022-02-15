package kaniko

import (
	"fmt"
	"os"

	"github.com/GoogleContainerTools/kaniko/pkg/config"
	"github.com/GoogleContainerTools/kaniko/pkg/executor"

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

	logger Logger
}

type Logger interface {
	Debug(msg string)
	Debugf(fmt string, v ...interface{})

	Info(msg string)
	Infof(fmt string, v ...interface{})

	Warn(msg string)
	Warnf(fmt string, v ...interface{})

	Error(msg string)
	Errorf(fmt string, v ...interface{})
}

func NewDockerfileApplier(cacheDir, contextDir, workDir string, logger Logger) *DockerfileApplier {
	return &DockerfileApplier{
		cacheDir:   cacheDir,
		contextDir: contextDir,
		logger:     logger,
		workDir:    workDir,
	}
}

func (a *DockerfileApplier) ApplyBuild(dockerfiles []extender.Dockerfile, image string, ignorePaths []string) error {
	registryHost := os.Getenv("REGISTRY_HOST") // TODO: formalize this in the "spec"

	destination := fmt.Sprintf("%s/extended/buildimage", registryHost)
	a.logger.Debugf("Destination Image: %s", destination)

	currentBaseImage := image
	for idx, dockerfile := range dockerfiles {
		if dockerfile.Type != buildKind {
			a.logger.Debugf("Skipping Dockerfile %s of wrong kind...", dockerfile.Path)
			continue
		}

		opts := config.KanikoOptions{
			BuildArgs:       append(toMultiArg(dockerfile.Args), fmt.Sprintf(`base_image=%s`, currentBaseImage)),
			Cleanup:         idx < len(dockerfiles)-1, // cleanup for all but the last dockerfile
			Destinations:    []string{destination},
			DockerfilePath:  dockerfile.Path,
			IgnoreVarRun:    true,                                        // TODO: add ignore paths
			RegistryOptions: config.RegistryOptions{SkipTLSVerify: true}, // TODO: remove eventually
			SnapshotMode:    "full",
			SrcContext:      a.workDir,
		}

		// TODO: link to kaniko code to show why this is necessary
		if err := os.Chdir("/"); err != nil {
			panic(err)
		}

		a.logger.Debugf("Applying the Dockerfile at %s...", dockerfile.Path)
		a.logger.Debugf("Options used: %+v", opts)
		newImage, err := executor.DoBuild(&opts)
		if err != nil {
			return err
		}
		a.logger.Debug("Pushing the image to its destination...")
		err = executor.DoPush(newImage, &opts)
		if err != nil {
			return err
		}

		// The base image for the next Dockerfile should be the one we just built
		currentBaseImage = destination // TODO: use digest instead
	}
	return nil
}

func (a *DockerfileApplier) ApplyRun(dockerfiles []extender.Dockerfile, image string, ignorePaths []string) error {
	registryHost := os.Getenv("REGISTRY_HOST") // TODO: formalize this in the "spec"

	destination := fmt.Sprintf("%s/extended/runimage", registryHost)
	a.logger.Debugf("Destination Image: %s", destination)

	currentBaseImage := image
	for _, dockerfile := range dockerfiles {
		if dockerfile.Type != runKind {
			a.logger.Debugf("Skipping Dockerfile %s of wrong kind...", dockerfile.Path)
			continue
		}

		opts := config.KanikoOptions{
			BuildArgs:       append(toMultiArg(dockerfile.Args), fmt.Sprintf(`base_image=%s`, currentBaseImage)),
			Cleanup:         true,
			Destinations:    []string{destination},
			DockerfilePath:  dockerfile.Path,
			IgnoreVarRun:    true,                                        // TODO: add ignore paths
			RegistryOptions: config.RegistryOptions{SkipTLSVerify: true}, // TODO: remove eventually
			SnapshotMode:    "full",
			SrcContext:      a.workDir,
		}

		// TODO: link to kaniko code to show why this is necessary
		if err := os.Chdir("/"); err != nil {
			panic(err)
		}

		a.logger.Debugf("Applying the Dockerfile at %s...", dockerfile.Path)
		a.logger.Debugf("Options used: %+v", opts)
		newImage, err := executor.DoBuild(&opts)
		if err != nil {
			return err
		}
		a.logger.Debug("Pushing the image to its destination...")
		err = executor.DoPush(newImage, &opts)
		if err != nil {
			return err
		}

		// The base image for the next Dockerfile should be the one we just built
		currentBaseImage = destination // TODO: use digest instead
	}
	a.logger.Debug("Done")
	return nil
}

func toMultiArg(args []extender.DockerfileArg) []string {
	var result []string
	for _, arg := range args {
		result = append(result, fmt.Sprintf("%s=%s", arg.Key, arg.Value))
	}
	return result
}
