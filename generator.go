package lifecycle

import (
	"io"
	"os"
	"path/filepath"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/internal/fsutil"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
)

type Generator struct {
	AppDir      string
	DirStore    DirStore
	Group       buildpack.Group
	OutputDir   string
	Logger      log.Logger
	Out, Err    io.Writer // TODO: does this need to be passed in explicitly?
	Plan        platform.BuildPlan
	PlatformDir string
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
	group buildpack.Group,
	outputDir string,
	plan platform.BuildPlan,
	platformDir string,
	logger log.Logger,
) (*Generator, error) {
	generator := &Generator{
		AppDir:      appDir,
		DirStore:    f.dirStore,
		OutputDir:   filepath.Join(outputDir, "generated"),
		Logger:      logger,
		Plan:        plan,
		PlatformDir: platformDir,
	}
	if err := f.setGroup(generator, group, logger); err != nil {
		return nil, err
	}
	return generator, nil
}

func (f *GeneratorFactory) setGroup(generator *Generator, group buildpack.Group, logger log.Logger) error {
	generator.Group = group.Filter(buildpack.KindExtension)
	for _, el := range generator.Group.Group {
		if err := f.apiVerifier.VerifyBuildpackAPI(el.Kind(), el.String(), el.API, logger); err != nil {
			return err
		}
	}
	return nil
}

func (g *Generator) Generate() (*platform.GeneratedMetadata, error) {
	var dockerfiles []buildpack.Dockerfile

	buildEnv := env.NewBuildEnv(os.Environ())
	plan := g.Plan
	for _, ext := range g.Group.Filter(buildpack.KindExtension).Group {
		g.Logger.Debugf("Running generate for extension %s", ext)

		g.Logger.Debug("Looking up module")
		descriptor, err := g.DirStore.Lookup(buildpack.KindExtension, ext.ID, ext.Version)
		if err != nil {
			return nil, err
		}

		g.Logger.Debug("Finding plan")
		foundPlan := plan.Find(ext)

		g.Logger.Debug("Invoking command")
		result, err := descriptor.Build(foundPlan, g.GenerateConfig(), buildEnv)
		if err != nil {
			return nil, err
		}

		// aggregate build results
		dockerfiles = append(dockerfiles, result.Dockerfiles...)
		plan = plan.Filter(result.MetRequires)

		g.Logger.Debugf("Finished running generate for extension %s", ext)
	}

	g.Logger.Debug("Copying Dockerfiles")
	var err error
	dockerfiles, err = g.copyDockerfiles(g.OutputDir, dockerfiles)
	if err != nil {
		return nil, err
	}

	g.Logger.Debug("Finished build")
	return &platform.GeneratedMetadata{
		Dockerfiles: dockerfiles,
	}, nil
}

func (g *Generator) GenerateConfig() buildpack.BuildConfig {
	return buildpack.BuildConfig{
		AppDir:          g.AppDir,
		OutputParentDir: g.OutputDir,
		PlatformDir:     g.PlatformDir,
		Out:             g.Out,
		Err:             g.Err,
		Logger:          g.Logger,
	}
}

func (g *Generator) copyDockerfiles(outputDir string, dockerfiles []buildpack.Dockerfile) ([]buildpack.Dockerfile, error) {
	var out []buildpack.Dockerfile
	for _, dockerfile := range dockerfiles {
		targetDir := filepath.Join(outputDir, launch.EscapeID(dockerfile.ExtensionID))
		targetPath := filepath.Join(targetDir, filepath.Base(dockerfile.Path))
		if dockerfile.Path == targetPath {
			out = append(out, dockerfile)
			continue
		}
		if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
			return nil, err
		}
		if err := fsutil.Copy(dockerfile.Path, targetPath); err != nil {
			return nil, err
		}
		dockerfile.Path = targetPath
		out = append(out, dockerfile)
	}
	return out, nil
}
