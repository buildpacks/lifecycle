package buildpack

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/BurntSushi/toml"
	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/log"
)

const (
	EnvBuildpackDir  = "CNB_BUILDPACK_DIR"
	EnvPlatformDir   = "CNB_PLATFORM_DIR"
	EnvBuildPlanPath = "CNB_BUILD_PLAN_PATH"
)

type DetectConfig struct {
	AppDir      string
	PlatformDir string
	Logger      log.Logger
}

func (d *Descriptor) Detect(config *DetectConfig, bpEnv BuildEnv) DetectRun {
	appDir, platformDir, err := processPlatformPaths(config)
	if err != nil {
		return DetectRun{Code: -1, Err: err}
	}

	planDir, planPath, err := processBuildpackPaths()
	defer os.RemoveAll(planDir)
	if err != nil {
		return DetectRun{Code: -1, Err: err}
	}

	var result DetectRun
	_, err = os.Stat(filepath.Join(d.Dir, "bin", "detect"))
	if d.IsExtension() && os.IsNotExist(err) {
		// treat extension root directory as pre-populated output directory
		planPath = filepath.Join(d.Dir, "detect", "plan.toml")
		if _, err := toml.DecodeFile(planPath, &result); err != nil && !os.IsNotExist(err) {
			return DetectRun{Code: -1, Err: err}
		}
	} else {
		result = d.runDetect(platformDir, planPath, appDir, bpEnv)
		if result.Code != 0 {
			return result
		}
		backupOut := result.Output
		if _, err := toml.DecodeFile(planPath, &result); err != nil {
			return DetectRun{Code: -1, Err: err, Output: backupOut}
		}
	}

	if api.MustParse(d.API).Equal(api.MustParse("0.2")) {
		if result.hasInconsistentVersions() || result.Or.hasInconsistentVersions() {
			result.Err = errors.Errorf(d.Kind()+` %s has a "version" key that does not match "metadata.version"`, d.Info().ID)
			result.Code = -1
		}
	}
	if api.MustParse(d.API).AtLeast("0.3") {
		if result.hasDoublySpecifiedVersions() || result.Or.hasDoublySpecifiedVersions() {
			result.Err = errors.Errorf(d.Kind()+` %s has a "version" key and a "metadata.version" which cannot be specified together. "metadata.version" should be used instead`, d.Info().ID)
			result.Code = -1
		}
	}
	if api.MustParse(d.API).AtLeast("0.3") {
		if result.hasTopLevelVersions() || result.Or.hasTopLevelVersions() {
			config.Logger.Warnf(d.Kind()+` %s has a "version" key. This key is deprecated in build plan requirements in buildpack API 0.3. "metadata.version" should be used instead`, d.Info().ID)
		}
	}
	if d.IsExtension() {
		if result.hasRequires() || result.Or.hasRequires() {
			result.Err = errors.Errorf(d.Kind()+` %s outputs "requires" which is not allowed`, d.Info().ID)
			result.Code = -1
		}
	}
	return result
}

func processPlatformPaths(config *DetectConfig) (string, string, error) {
	appDir, err := filepath.Abs(config.AppDir)
	if err != nil {
		return "", "", nil
	}
	platformDir, err := filepath.Abs(config.PlatformDir)
	if err != nil {
		return "", "", nil
	}
	return appDir, platformDir, nil
}

func processBuildpackPaths() (string, string, error) {
	planDir, err := ioutil.TempDir("", "plan.")
	if err != nil {
		return "", "", err
	}
	planPath := filepath.Join(planDir, "plan.toml")
	if err = ioutil.WriteFile(planPath, nil, 0600); err != nil {
		return "", "", err
	}
	return planDir, planPath, nil
}

func (d *Descriptor) runDetect(platformDir, planPath, appDir string, bpEnv BuildEnv) DetectRun {
	out := &bytes.Buffer{}
	cmd := exec.Command(
		filepath.Join(d.Dir, "bin", "detect"),
		platformDir,
		planPath,
	) // #nosec G204
	cmd.Dir = appDir
	cmd.Stdout = out
	cmd.Stderr = out

	var err error
	if d.Info().ClearEnv {
		cmd.Env = bpEnv.List()
	} else {
		cmd.Env, err = bpEnv.WithPlatform(platformDir)
		if err != nil {
			return DetectRun{Code: -1, Err: err}
		}
	}
	cmd.Env = append(cmd.Env, EnvBuildpackDir+"="+d.Dir)
	if api.MustParse(d.API).AtLeast("0.8") {
		cmd.Env = append(
			cmd.Env,
			EnvPlatformDir+"="+platformDir,
			EnvBuildPlanPath+"="+planPath,
		)
	}

	if err := cmd.Run(); err != nil {
		if err, ok := err.(*exec.ExitError); ok {
			if status, ok := err.Sys().(syscall.WaitStatus); ok {
				return DetectRun{Code: status.ExitStatus(), Output: out.Bytes()}
			}
		}
		return DetectRun{Code: -1, Err: err, Output: out.Bytes()}
	}
	return DetectRun{Code: 0, Err: nil, Output: out.Bytes()}
}

type DetectRun struct {
	BuildPlan
	Output []byte `toml:"-"`
	Code   int    `toml:"-"`
	Err    error  `toml:"-"`
}
