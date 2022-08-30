package buildpack

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/buildpacks/lifecycle/launch"
)

const generateCmd = "generate"

//go:generate mockgen -package testmock -destination ../testmock/generate_executor.go github.com/buildpacks/lifecycle/buildpack GenerateExecutor
type GenerateExecutor interface {
	Generate(d ExtDescriptor, plan Plan, config BuildConfig, buildEnv BuildEnv) (BuildResult, error)
}

type DefaultGenerateExecutor struct{}

func (e *DefaultGenerateExecutor) Generate(d ExtDescriptor, plan Plan, config BuildConfig, buildEnv BuildEnv) (BuildResult, error) { // TODO: fix other pointer arguments (Build, Detect)
	config.Logger.Debug("Creating plan directory")
	planDir, err := ioutil.TempDir("", launch.EscapeID(d.Extension.ID)+"-")
	if err != nil {
		return BuildResult{}, err
	}
	defer os.RemoveAll(planDir)

	config.Logger.Debug("Preparing paths")
	moduleOutputDir, planPath, err := prepareBuildInputPaths(d.Extension.ID, plan, config.OutputParentDir, planDir)
	if err != nil {
		return BuildResult{}, err
	}

	config.Logger.Debug("Running generate command")
	if _, err = os.Stat(filepath.Join(d.WithRootDir, "bin", "generate")); err != nil {
		if os.IsNotExist(err) {
			// treat extension root directory as pre-populated output directory
			return readOutputFilesExt(d, filepath.Join(d.WithRootDir, "generate"), plan)
		}
		return BuildResult{}, err
	}
	if err = runGenerateCmd(d, moduleOutputDir, planPath, config, buildEnv); err != nil {
		return BuildResult{}, err
	}

	config.Logger.Debug("Reading output files")
	return readOutputFilesExt(d, moduleOutputDir, plan)
}

func runGenerateCmd(d ExtDescriptor, moduleOutputDir, planPath string, config BuildConfig, env BuildEnv) error {
	cmd := exec.Command(
		filepath.Join(d.WithRootDir, "bin", generateCmd),
		moduleOutputDir,
		config.PlatformDir,
		planPath,
	) // #nosec G204
	cmd.Dir = config.AppDir
	cmd.Stdout = config.Out
	cmd.Stderr = config.Err

	var err error
	if d.Extension.ClearEnv {
		cmd.Env = env.List()
	} else {
		cmd.Env, err = env.WithPlatform(config.PlatformDir)
		if err != nil {
			return err
		}
	}
	cmd.Env = append(cmd.Env,
		EnvBpPlanPath+"="+planPath,
		EnvBuildpackDir+"="+d.WithRootDir, // TODO: should be extension dir?
		EnvOutputDir+"="+moduleOutputDir,
		EnvPlatformDir+"="+config.PlatformDir,
	)

	if err := cmd.Run(); err != nil {
		return NewError(err, ErrTypeBuildpack)
	}
	return nil
}

func readOutputFilesExt(d ExtDescriptor, extOutputDir string, extPlanIn Plan) (BuildResult, error) {
	br := BuildResult{}
	var err error

	// set MetRequires
	br.MetRequires = names(extPlanIn.Entries)

	// set Dockerfiles
	runDockerfile := filepath.Join(extOutputDir, "run.Dockerfile")
	if _, err = os.Stat(runDockerfile); err != nil {
		if os.IsNotExist(err) {
			return br, nil
		}
		return BuildResult{}, err
	}
	br.Dockerfiles = []DockerfileInfo{{ExtensionID: d.Extension.ID, Kind: DockerfileKindRun, Path: runDockerfile}}
	return br, nil
}
