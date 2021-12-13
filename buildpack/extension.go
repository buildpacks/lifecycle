package buildpack

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"
)

type Extension struct {
	API       string `toml:"api"`
	Buildpack Info   `toml:"buildpack"`
	Dir       string `toml:"-"`
}

func (e *Extension) ConfigFile() *Extension {
	return e
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
	br := BuildResult{}

	// read build.toml
	type extBuildTOML struct {
		Unmet []Unmet `toml:"unmet"`
	}
	var bpBuild extBuildTOML
	buildPath := filepath.Join(bpOutputDir, "build.toml")
	if _, err := toml.DecodeFile(buildPath, &bpBuild); err != nil && !os.IsNotExist(err) {
		return BuildResult{}, err
	}

	// set MetRequires
	if err := validateUnmet(bpBuild.Unmet, bpPlanIn); err != nil {
		return BuildResult{}, err
	}
	br.MetRequires = names(bpPlanIn.filter(bpBuild.Unmet).Entries)

	// set BOM files
	// br.BOMFiles, err = b.processBOMFiles(bpOutputDir, bpFromBpInfo, bpLayers, logger) // TODO: extract service
	// if err != nil {
	// 	return BuildResult{}, err
	// }

	// read launch.toml
	type extLaunchTOML struct {
		Args   []DockerfileBuildArg // TODO: fix
		Labels []Label
	}
	var launchTOML extLaunchTOML
	launchPath := filepath.Join(bpOutputDir, "launch.toml")
	if _, err := toml.DecodeFile(launchPath, &launchTOML); !os.IsNotExist(err) {
		return BuildResult{}, err
	}

	return br, nil
}
