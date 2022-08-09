package lifecycle

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/dockerfile"
	"github.com/buildpacks/lifecycle/internal/dockerfile/kaniko"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/log"
)

type Extender struct {
	Extensions   []buildpack.GroupElement
	GeneratedDir string
	Logger       log.Logger
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

func (f *ExtenderFactory) NewExtender(extensions []buildpack.GroupElement, groupPath string, generatedDir string, logger log.Logger) (*Extender, error) {
	extender := &Extender{
		GeneratedDir: generatedDir,
		Logger:       logger,
	}
	if err := f.setExtensions(extender, extensions, groupPath, logger); err != nil {
		return nil, err
	}
	return extender, nil
}

func (f *ExtenderFactory) setExtensions(extender *Extender, extensions []buildpack.GroupElement, groupPath string, logger log.Logger) error {
	if len(extensions) > 0 {
		extender.Extensions = extensions
	}
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

func (e *Extender) LastRunImage() (string, error) {
	lastExtension := e.Extensions[len(e.Extensions)-1] // TODO: work backward through extensions until there is a Dockerfile in run
	contents, err := ioutil.ReadFile(filepath.Join(e.GeneratedDir, "run", lastExtension.ID, "Dockerfile"))
	if err != nil {
		return "", err
	}
	strContents := string(contents)
	parts := strings.Split(strContents, " ")
	if len(parts) != 2 || parts[0] != "FROM" {
		return "", fmt.Errorf("failed to parse dockerfile, expected format 'FROM <image>', got: '%s'", strContents)
	}
	return strings.TrimSpace(parts[1]), nil
}

func (e *Extender) ExtendBuild(imageRef string) error {
	e.Logger.Debugf("Extending %s", imageRef)
	applier := kaniko.NewDockerfileApplier()
	var dockerfiles []dockerfile.Dockerfile
	for _, ext := range e.Extensions {
		buildDockerfile, exists := e.buildDockerfileFor(ext.ID)
		if exists {
			e.Logger.Debugf("Found build Dockerfile for extension '%s'", ext.ID)
			dockerfiles = append(dockerfiles, buildDockerfile)
		}
	}
	return applier.Apply(dockerfiles, imageRef, e.Logger) // TODO: make this configurable?
}

func (e *Extender) buildDockerfileFor(extID string) (dockerfile.Dockerfile, bool) {
	dockerfilePath := filepath.Join(e.GeneratedDir, "build", launch.EscapeID(extID), "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		return dockerfile.Dockerfile{}, false
	}
	return dockerfile.Dockerfile{
		Path: dockerfilePath, // TODO: read args from somewhere
	}, true
}
