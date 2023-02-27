package lifecycle

import (
	"fmt"
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
	AppDir         string
	BuildConfigDir string
	GeneratedDir   string // e.g., <layers>/generated
	PlatformDir    string
	AnalyzedMD     platform.AnalyzedMetadata
	DirStore       DirStore
	Executor       buildpack.GenerateExecutor
	Extensions     []buildpack.GroupElement
	Logger         log.Logger
	Out, Err       io.Writer
	Plan           platform.BuildPlan
	RunMetadata    platform.RunMetadata
}

type GeneratorFactory struct {
	apiVerifier   BuildpackAPIVerifier
	configHandler ConfigHandler
	dirStore      DirStore
}

func NewGeneratorFactory(
	apiVerifier BuildpackAPIVerifier,
	configHandler ConfigHandler,
	dirStore DirStore,
) *GeneratorFactory {
	return &GeneratorFactory{
		apiVerifier:   apiVerifier,
		configHandler: configHandler,
		dirStore:      dirStore,
	}
}

func (f *GeneratorFactory) NewGenerator(
	analyzedPath string,
	appDir string,
	buildConfigDir string,
	extensions []buildpack.GroupElement,
	generatedDir string,
	plan platform.BuildPlan,
	platformDir string,
	runPath string,
	stdout, stderr io.Writer,
	logger log.Logger,
) (*Generator, error) {
	generator := &Generator{
		AppDir:         appDir,
		BuildConfigDir: buildConfigDir,
		GeneratedDir:   generatedDir,
		PlatformDir:    platformDir,
		DirStore:       f.dirStore,
		Executor:       &buildpack.DefaultGenerateExecutor{},
		Logger:         logger,
		Plan:           plan,
		Out:            stdout,
		Err:            stderr,
	}

	if err := f.setExtensions(generator, extensions, logger); err != nil {
		return nil, err
	}
	if err := f.setAnalyzedMD(generator, analyzedPath); err != nil {
		return nil, err
	}
	if err := f.setRunMD(generator, runPath, logger); err != nil {
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

func (f *GeneratorFactory) setAnalyzedMD(generator *Generator, analyzedPath string) error {
	var err error
	generator.AnalyzedMD, err = f.configHandler.ReadAnalyzed(analyzedPath)
	return err
}

func (f *GeneratorFactory) setRunMD(generator *Generator, runPath string, logger log.Logger) error {
	var err error
	generator.RunMetadata, err = f.configHandler.ReadRun(runPath, logger)
	return err
}

type GenerateResult struct {
	AnalyzedMD platform.AnalyzedMetadata
	Plan       platform.BuildPlan
	UsePlan    bool
}

func (g *Generator) Generate() (GenerateResult, error) {
	inputs := g.getGenerateInputs()
	extensionOutputParentDir, err := os.MkdirTemp("", "cnb-extensions-generated.")
	if err != nil {
		return GenerateResult{}, err
	}
	defer os.RemoveAll(extensionOutputParentDir)
	inputs.OutputDir = extensionOutputParentDir

	var dockerfiles []buildpack.DockerfileInfo
	filteredPlan := g.Plan
	for _, ext := range g.Extensions {
		g.Logger.Debugf("Running generate for extension %s", ext)

		g.Logger.Debug("Looking up extension")
		descriptor, err := g.DirStore.LookupExt(ext.ID, ext.Version)
		if err != nil {
			return GenerateResult{}, err
		}

		g.Logger.Debug("Finding plan")
		inputs.Plan = filteredPlan.Find(buildpack.KindExtension, ext.ID)

		g.Logger.Debug("Invoking command")
		result, err := g.Executor.Generate(*descriptor, inputs, g.Logger)
		if err != nil {
			return GenerateResult{}, err
		}

		// aggregate build results
		dockerfiles = append(dockerfiles, result.Dockerfiles...)
		filteredPlan = filteredPlan.Filter(result.MetRequires)

		g.Logger.Debugf("Finished running generate for extension %s", ext)
	}

	g.Logger.Debug("Checking for new run image")
	base, newBaseIdx, extend := g.checkNewRunImage(dockerfiles)
	if err != nil {
		return GenerateResult{}, err
	}
	if !containsMatch(g.RunMetadata.Images, base) {
		return GenerateResult{}, fmt.Errorf("new runtime base image '%s' not found in run metadata", base)
	}

	g.Logger.Debug("Copying Dockerfiles")
	if err = g.copyDockerfiles(dockerfiles, newBaseIdx); err != nil {
		return GenerateResult{}, err
	}

	newAnalyzedMD := g.AnalyzedMD
	if newRunImage(base, g.AnalyzedMD) {
		g.Logger.Debugf("Updating analyzed metadata with new run image '%s'", base)
		newAnalyzedMD.RunImage = &platform.RunImage{ // target data is cleared
			Reference: base,
			Extend:    extend,
		}
	} else if extend && g.AnalyzedMD.RunImage != nil {
		g.Logger.Debug("Updating analyzed metadata with run image extend")
		newAnalyzedMD.RunImage.Extend = true
	}

	return GenerateResult{
		AnalyzedMD: newAnalyzedMD,
		Plan:       filteredPlan,
		UsePlan:    true,
	}, nil
}

func containsMatch(images []platform.RunImageMetadata, imageName string) bool {
	if len(images) == 0 {
		// if no run image metadata was provided, consider it a match
		return true
	}
	for _, image := range images {
		if image.Image == imageName {
			return true
		}
	}
	return false
}

func (g *Generator) getGenerateInputs() buildpack.GenerateInputs {
	return buildpack.GenerateInputs{
		AppDir:         g.AppDir,
		BuildConfigDir: g.BuildConfigDir,
		PlatformDir:    g.PlatformDir,
		Env:            env.NewBuildEnv(os.Environ()),
		Out:            g.Out,
		Err:            g.Err,
	}
}

func (g *Generator) copyDockerfiles(dockerfiles []buildpack.DockerfileInfo, newBaseIdx int) error {
	for currentIdx, dockerfile := range dockerfiles {
		targetDir := filepath.Join(g.GeneratedDir, dockerfile.Kind, launch.EscapeID(dockerfile.ExtensionID))
		var targetPath = filepath.Join(targetDir, "Dockerfile")
		if dockerfile.Kind == buildpack.DockerfileKindRun && currentIdx < newBaseIdx {
			targetPath += ".ignore"
		}
		if err := os.MkdirAll(targetDir, os.ModePerm); err != nil {
			return err
		}
		g.Logger.Debugf("Copying %s to %s", dockerfile.Path, targetPath)
		if err := fsutil.Copy(dockerfile.Path, targetPath); err != nil {
			return err
		}
		// check for extend-config.toml and if found, copy
		extendConfigPath := filepath.Join(filepath.Dir(dockerfile.Path), "extend-config.toml")
		if err := fsutil.Copy(extendConfigPath, filepath.Join(targetDir, "extend-config.toml")); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
		}
	}
	return nil
}

func (g *Generator) checkNewRunImage(dockerfiles []buildpack.DockerfileInfo) (newBase string, newBaseIdx int, extend bool) {
	// There may be extensions that contribute only a build.Dockerfile; work backward through extensions until we find
	// a run.Dockerfile.
	for i := len(dockerfiles) - 1; i >= 0; i-- {
		if dockerfiles[i].Kind != buildpack.DockerfileKindRun {
			continue
		}
		if dockerfiles[i].Base != "" {
			newBase = dockerfiles[i].Base
			newBaseIdx = i
			g.Logger.Debugf("Found a run.Dockerfile configuring image '%s' from extension with id '%s'", newBase, dockerfiles[i].ExtensionID)
			break
		}
		if dockerfiles[i].Base == "" {
			extend = true
		}
	}
	return newBase, newBaseIdx, extend
}

func newRunImage(base string, analyzedMD platform.AnalyzedMetadata) bool {
	if base == "" {
		return false
	}
	if analyzedMD.RunImage == nil {
		return true
	}
	return base != analyzedMD.RunImage.Reference
}
