package lifecycle

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/extend"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/log"
)

type Extender struct {
	Extensions   []buildpack.GroupElement
	GeneratedDir string
	Logger       log.Logger

	DockerfileApplier DockerfileApplier
}

//go:generate mockgen -package testmock -destination testmock/dockerfile_applier.go github.com/buildpacks/lifecycle DockerfileApplier
type DockerfileApplier interface {
	Apply(dockerfiles []extend.Dockerfile, baseImageRef string, logger log.Logger) error
}

type ExtenderFactory struct {
	apiVerifier   BuildpackAPIVerifier
	configHandler ConfigHandler
}

func NewExtenderFactory(apiVerifier BuildpackAPIVerifier, configHandler ConfigHandler) *ExtenderFactory {
	return &ExtenderFactory{
		apiVerifier:   apiVerifier,
		configHandler: configHandler,
	}
}

func (f *ExtenderFactory) NewExtender(dockerfileApplier DockerfileApplier, groupPath string, generatedDir string, logger log.Logger) (*Extender, error) {
	extender := &Extender{
		GeneratedDir:      generatedDir,
		Logger:            logger,
		DockerfileApplier: dockerfileApplier,
	}
	if err := f.setExtensions(extender, groupPath, logger); err != nil {
		return nil, err
	}
	return extender, nil
}

func (f *ExtenderFactory) setExtensions(extender *Extender, groupPath string, logger log.Logger) error {
	var err error
	if _, extender.Extensions, err = f.configHandler.ReadGroup(groupPath); err != nil {
		return err
	}
	for _, el := range extender.Extensions {
		if err := f.apiVerifier.VerifyBuildpackAPI(buildpack.KindExtension, el.String(), el.API, logger); err != nil {
			return err
		}
	}
	return nil
}

func (e *Extender) ExtendBuild(imageRef string) error {
	e.Logger.Debugf("Extending %s", imageRef)
	var dockerfiles []extend.Dockerfile
	for _, ext := range e.Extensions {
		buildDockerfile, err := e.buildDockerfileFor(ext.ID)
		if err != nil {
			return err
		}
		if buildDockerfile != nil {
			e.Logger.Debugf("Found build Dockerfile for extension '%s'", ext.ID)
			dockerfiles = append(dockerfiles, *buildDockerfile)
		}
	}
	return e.DockerfileApplier.Apply(dockerfiles, imageRef, e.Logger)
}

func (e *Extender) buildDockerfileFor(extID string) (*extend.Dockerfile, error) {
	dockerfilePath := filepath.Join(e.GeneratedDir, "build", launch.EscapeID(extID), "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		return nil, nil
	}

	configPath := filepath.Join(e.GeneratedDir, "build", launch.EscapeID(extID), "extend-config.toml")
	var config buildpack.ExtendConfig
	_, err := toml.DecodeFile(configPath, &config)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	var args []extend.Arg
	for _, configArg := range config.Build.Args {
		args = append(args, extend.Arg{
			Name:  configArg.Name,
			Value: configArg.Value,
		})
	}

	return &extend.Dockerfile{
		Path: dockerfilePath,
		Args: args,
	}, nil
}
