package buildpack

import (
	"os"
	"path/filepath"

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

	config.Logger.Debug("Processing output directory")
	config.Logger.Debug("Reading output files")
	return BuildResult{}, nil
}
