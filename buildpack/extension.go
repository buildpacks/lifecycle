package buildpack

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

type Extension struct {
	API        string `toml:"api"`
	Buildpack  Info   `toml:"extension"`
	Dir        string `toml:"-"`
	Descriptor *Descriptor
}

func (e *Extension) ConfigFile() *Descriptor {
	if e.Descriptor == nil {
		e.Descriptor = &Descriptor{
			API:       e.API,
			Buildpack: e.Buildpack,
			Order:     nil, // TODO: check
			Dir:       e.Dir,
		}
	}
	return e.Descriptor
}

func (e *Extension) IsMetaBuildpack() bool {
	return false
}

func (e *Extension) String() string {
	return e.Buildpack.Name + " " + e.Buildpack.Version
}

func (e *Extension) Detect(config *DetectConfig, bpEnv BuildEnv) DetectRun {
	if _, err := os.Stat(filepath.Join(e.Dir, "bin", "detect")); os.IsNotExist(err) {
		config.Logger.Debugf("Passing detect due to missing %s", filepath.Join("bin", "detect"))
		return DetectRun{}
	}
	t := doDetect(config, bpEnv, e.Dir, e.Buildpack.ClearEnv)
	if t.hasRequires() || t.Or.hasRequires() {
		t.Err = errors.Errorf(`extension %s outputs "requires" to the build plan; only "provides" are permitted`, e.Buildpack.ID)
		t.Code = -1
	}
	return t
}

func (e *Extension) Build(bpPlan Plan, config BuildConfig, bpEnv BuildEnv) (BuildResult, error) {
	config.Logger.Debugf("Running build for extension %s", e)

	if _, err := os.Stat(filepath.Join(e.Dir, "bin", "build")); os.IsNotExist(err) {
		config.Logger.Debugf("Passing build due to missing %s", filepath.Join("bin", "build"))
		config.Logger.Debug("Reading pre-populated files")
		return e.readPrePopulatedFiles(e.Dir, config.Logger)
	}

	config.Logger.Debug("Preparing paths")
	bpPlanDir, bpOutputDir, bpPlanPath, err := prepareBuildPaths(config.LayersDir, e.Buildpack.ID, bpPlan)
	defer os.RemoveAll(bpPlanDir)
	if err != nil {
		return BuildResult{}, err
	}

	config.Logger.Debug("Running build command")
	if err := runBuildCmd(e.Dir, bpOutputDir, bpPlanPath, config, bpEnv, e.Buildpack.ClearEnv); err != nil {
		return BuildResult{}, err
	}

	config.Logger.Debug("Reading output files")
	return e.readOutputFiles(bpOutputDir, bpPlan, config.Logger)
}

func (e *Extension) readOutputFiles(bpOutputDir string, bpPlanIn Plan, logger Logger) (BuildResult, error) {
	br := &BuildResult{}

	err := readExtBuildTOML(filepath.Join(bpOutputDir, "build.toml"), br, bpPlanIn)
	if err != nil {
		return BuildResult{}, errors.Wrap(err, "reading build.toml")
	}

	err = readExtLaunchTOML(filepath.Join(bpOutputDir, "launch.toml"), br)
	if err != nil {
		return BuildResult{}, errors.Wrap(err, "reading launch.toml")
	}

	dockerfiles, err := processDockerfiles(bpOutputDir)
	if err != nil {
		return BuildResult{}, errors.Wrap(err, "processing Dockerfiles")
	}
	br.Dockerfiles = dockerfiles

	// set BOM files
	// br.BOMFiles, err = b.processBOMFiles(bpOutputDir, bpFromBpInfo, bpLayers, logger) // TODO: extract service
	// if err != nil {
	// 	return BuildResult{}, err
	// }

	return *br, nil
}

func (e *Extension) readPrePopulatedFiles(bpOutputDir string, logger Logger) (BuildResult, error) {
	br := &BuildResult{}

	dockerfiles, err := processDockerfiles(bpOutputDir)
	if err != nil {
		return BuildResult{}, errors.Wrap(err, "processing Dockerfiles")
	}
	br.Dockerfiles = dockerfiles

	// set BOM files
	// br.BOMFiles, err = b.processBOMFiles(bpOutputDir, bpFromBpInfo, bpLayers, logger) // TODO: extract service
	// if err != nil {
	// 	return BuildResult{}, err
	// }

	return *br, nil
}

func readExtBuildTOML(path string, br *BuildResult, bpPlanIn Plan) error {
	type extBuildTOML struct {
		Args  []DockerfileArg `toml:"args"`
		Unmet []Unmet         `toml:"unmet"`
	}
	var buildTOML extBuildTOML
	if _, err := toml.DecodeFile(path, &buildTOML); err != nil && !os.IsNotExist(err) {
		return err
	}
	br.DockerfileArgsForBuildExt = buildTOML.Args
	if err := validateUnmet(buildTOML.Unmet, bpPlanIn); err != nil {
		return err
	}
	br.MetRequires = names(bpPlanIn.filter(buildTOML.Unmet).Entries)
	return nil
}

func readExtLaunchTOML(path string, br *BuildResult) error {
	type extLaunchTOML struct {
		Args   []DockerfileArg `toml:"args"`
		Labels []Label         `toml:"labels"`
	}
	var launchTOML extLaunchTOML
	if _, err := toml.DecodeFile(path, &launchTOML); err != nil && !os.IsNotExist(err) {
		return err
	}
	br.DockerfileArgsForRunExt = launchTOML.Args
	return nil
}
