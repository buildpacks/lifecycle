package lifecycle

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/internal/fsutil"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
)

type Generator struct {
	AppDir         string
	DirStore       DirStore
	Extensions     []buildpack.GroupElement
	OutputDir      string
	Logger         log.Logger
	Stdout, Stderr io.Writer
	Plan           platform.BuildPlan
	PlatformDir    string
}

type GeneratorFactory struct {
	apiVerifier BuildpackAPIVerifier
	dirStore    DirStore
}

func NewGeneratorFactory(
	apiVerifier BuildpackAPIVerifier,
	dirStore DirStore,
) *GeneratorFactory {
	return &GeneratorFactory{
		apiVerifier: apiVerifier,
		dirStore:    dirStore,
	}
}

func (f *GeneratorFactory) NewGenerator(
	appDir string,
	extensions []buildpack.GroupElement,
	outputDir string,
	plan platform.BuildPlan,
	platformDir string,
	stdout, stderr io.Writer,
	logger log.Logger,
) (*Generator, error) {
	generator := &Generator{
		AppDir:      appDir,
		DirStore:    f.dirStore,
		Logger:      logger,
		OutputDir:   outputDir,
		Plan:        plan,
		PlatformDir: platformDir,
		Stdout:      stdout,
		Stderr:      stderr,
	}
	if err := f.setExtensions(generator, extensions, logger); err != nil {
		return nil, err
	}
	return generator, nil
}

func (f *GeneratorFactory) setExtensions(generator *Generator, extensions []buildpack.GroupElement, logger log.Logger) error {
	generator.Extensions = extensions
	for _, el := range generator.Extensions {
		if err := f.apiVerifier.VerifyBuildpackAPI(buildpack.KindExtension, el.String(), el.API, logger); err != nil {
			return err
		}
	}
	return nil
}

type GenerateResult struct {
	RunImage string
}

func (g *Generator) Generate() (GenerateResult, error) {
	var dockerfiles []buildpack.Dockerfile

	config := g.GenerateConfig()
	tempOutputDir, err := ioutil.TempDir("", "generated.")
	if err != nil {
		return GenerateResult{}, err
	}
	defer os.RemoveAll(tempOutputDir)
	config.OutputParentDir = tempOutputDir

	buildEnv := env.NewBuildEnv(os.Environ())
	plan := g.Plan
	for _, ext := range g.Extensions {
		g.Logger.Debugf("Running generate for extension %s", ext)

		g.Logger.Debug("Looking up module")
		descriptor, err := g.DirStore.Lookup(buildpack.KindExtension, ext.ID, ext.Version)
		if err != nil {
			return GenerateResult{}, err
		}

		g.Logger.Debug("Finding plan")
		foundPlan := plan.Find(buildpack.KindExtension, ext.ID)

		g.Logger.Debug("Invoking command")
		result, err := descriptor.Build(foundPlan, config, buildEnv)
		if err != nil {
			return GenerateResult{}, err
		}

		// aggregate build results
		dockerfiles = append(dockerfiles, result.Dockerfiles...)
		plan = plan.Filter(result.MetRequires)

		g.Logger.Debugf("Finished running generate for extension %s", ext)
	}

	g.Logger.Debug("Copying Dockerfiles")
	if err := g.copyDockerfiles(dockerfiles); err != nil {
		return GenerateResult{}, err
	}

	g.Logger.Debug("Checking for new run image")
	runImage, err := g.checkNewRunImage()
	if err != nil {
		return GenerateResult{}, err
	}

	g.Logger.Debug("Finished build")
	return GenerateResult{RunImage: runImage}, nil
}

func (g *Generator) GenerateConfig() buildpack.BuildConfig {
	return buildpack.BuildConfig{
		AppDir:      g.AppDir,
		Err:         g.Stderr,
		Logger:      g.Logger,
		Out:         g.Stdout,
		PlatformDir: g.PlatformDir,
	}
}

func (g *Generator) copyDockerfiles(dockerfiles []buildpack.Dockerfile) error {
	for _, dockerfile := range dockerfiles {
		targetDir := filepath.Join(g.OutputDir, dockerfile.Kind, launch.EscapeID(dockerfile.ExtensionID))
		targetPath := filepath.Join(targetDir, "Dockerfile")
		if dockerfile.Path == targetPath {
			continue
		}
		if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
			return err
		}
		if err := fsutil.Copy(dockerfile.Path, targetPath); err != nil {
			return err
		}
	}
	return nil
}

func (g *Generator) checkNewRunImage() (string, error) {
	// There may be extensions that contribute only a build.Dockerfile; work backward through extensions until we find
	// a run.Dockerfile.
	for i := len(g.Extensions) - 1; i >= 0; i-- {
		extID := g.Extensions[i].ID
		runDockerfile := filepath.Join(g.OutputDir, "run", extID, "Dockerfile")
		if _, err := os.Stat(runDockerfile); os.IsNotExist(err) {
			continue
		}
		contents, err := ioutil.ReadFile(runDockerfile)
		if err != nil {
			return "", err
		}
		strContents := string(contents)
		parts := strings.Split(strContents, " ")
		if len(parts) != 2 || parts[0] != "FROM" {
			return "", fmt.Errorf("failed to parse Dockerfile, expected format 'FROM <image>', got: '%s'", strContents)
		}
		return strings.TrimSpace(parts[1]), nil
	}
	return "", nil
}
