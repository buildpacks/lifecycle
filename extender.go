package lifecycle

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/log"
)

type Extender struct {
	Extensions   []buildpack.GroupElement
	GeneratedDir string
	Logger       log.Logger
}

type ExtenderFactory struct {
	apiVerifier BuildpackAPIVerifier
	dirStore    DirStore
}

func NewExtenderFactory(
	apiVerifier BuildpackAPIVerifier,
	dirStore DirStore,
) *ExtenderFactory {
	return &ExtenderFactory{
		apiVerifier: apiVerifier,
		dirStore:    dirStore,
	}
}

func (f *ExtenderFactory) NewExtender(
	extensions []buildpack.GroupElement,
	generatedDir string,
	logger log.Logger,
) (*Extender, error) {
	generator := &Extender{
		GeneratedDir: generatedDir,
		Logger:       logger,
	}
	if err := f.setExtensions(generator, extensions, logger); err != nil {
		return nil, err
	}
	return generator, nil
}

func (f *ExtenderFactory) setExtensions(extender *Extender, extensions []buildpack.GroupElement, logger log.Logger) error {
	extender.Extensions = extensions
	for _, el := range extender.Extensions {
		if err := f.apiVerifier.VerifyBuildpackAPI(buildpack.KindExtension, el.String(), el.API, logger); err != nil {
			return err
		}
	}
	return nil
}

func (e *Extender) LastRunImage() (string, error) {
	lastExtension := e.Extensions[len(e.Extensions)-1]
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
