package buildpack

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/BurntSushi/toml"

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

//go:generate mockgen -package testmock -destination ../testmock/detect_executor.go github.com/buildpacks/lifecycle/buildpack DetectExecutor
type DetectExecutor interface {
	Detect(d Descriptor, config *DetectConfig, bpEnv BuildEnv) DetectRun
}

type DefaultDetectExecutor struct{}

func (e *DefaultDetectExecutor) Detect(d Descriptor, config *DetectConfig, bpEnv BuildEnv) DetectRun {
	switch descriptor := d.(type) {
	case *BpDescriptor:
		return detectBp(descriptor, config, bpEnv)
	case *ExtDescriptor:
		return detectExt(descriptor, config, bpEnv)
	default:
		return DetectRun{Code: -1, Err: fmt.Errorf("unknown descriptor type: %t", descriptor)}
	}
}

func detectBp(d *BpDescriptor, config *DetectConfig, env BuildEnv) DetectRun {
	appDir, platformDir, err := processPlatformPaths(config)
	if err != nil {
		return DetectRun{Code: -1, Err: err}
	}

	planDir, planPath, err := processBuildpackPaths()
	defer os.RemoveAll(planDir)
	if err != nil {
		return DetectRun{Code: -1, Err: err}
	}

	result := runDetect(d, platformDir, planPath, appDir, env)
	if result.Code != 0 {
		return result
	}
	backupOut := result.Output
	if _, err := toml.DecodeFile(planPath, &result); err != nil {
		return DetectRun{Code: -1, Err: err, Output: backupOut}
	}

	if api.MustParse(d.WithAPI).Equal(api.MustParse("0.2")) {
		if result.hasInconsistentVersions() || result.Or.hasInconsistentVersions() {
			result.Err = fmt.Errorf(`buildpack %s has a "version" key that does not match "metadata.version"`, d.Buildpack.ID)
			result.Code = -1
		}
	}
	if api.MustParse(d.WithAPI).AtLeast("0.3") {
		if result.hasDoublySpecifiedVersions() || result.Or.hasDoublySpecifiedVersions() {
			result.Err = fmt.Errorf(`buildpack %s has a "version" key and a "metadata.version" which cannot be specified together. "metadata.version" should be used instead`, d.Buildpack.ID)
			result.Code = -1
		}
	}
	if api.MustParse(d.WithAPI).AtLeast("0.3") {
		if result.hasTopLevelVersions() || result.Or.hasTopLevelVersions() {
			config.Logger.Warnf(`buildpack %s has a "version" key. This key is deprecated in build plan requirements in buildpack API 0.3. "metadata.version" should be used instead`, d.Buildpack.ID)
		}
	}

	return result
}

func detectExt(d *ExtDescriptor, config *DetectConfig, env BuildEnv) DetectRun {
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
	_, err = os.Stat(filepath.Join(d.WithRootDir, "bin", "detect"))
	if os.IsNotExist(err) {
		// treat extension root directory as pre-populated output directory
		planPath = filepath.Join(d.WithRootDir, "detect", "plan.toml")
		if _, err := toml.DecodeFile(planPath, &result); err != nil && !os.IsNotExist(err) {
			return DetectRun{Code: -1, Err: err}
		}
	} else {
		result = runDetect(d, platformDir, planPath, appDir, env)
		if result.Code != 0 {
			return result
		}
		backupOut := result.Output
		if _, err := toml.DecodeFile(planPath, &result); err != nil {
			return DetectRun{Code: -1, Err: err, Output: backupOut}
		}
	}

	if result.hasDoublySpecifiedVersions() || result.Or.hasDoublySpecifiedVersions() {
		result.Err = fmt.Errorf(`extension %s has a "version" key and a "metadata.version" which cannot be specified together. "metadata.version" should be used instead`, d.Extension.ID)
		result.Code = -1
	}
	if result.hasTopLevelVersions() || result.Or.hasTopLevelVersions() {
		config.Logger.Warnf(`extension %s has a "version" key. This key is deprecated in build plan requirements in buildpack API 0.3. "metadata.version" should be used instead`, d.Extension.ID)
	}
	if result.hasRequires() || result.Or.hasRequires() {
		result.Err = fmt.Errorf(`extension %s outputs "requires" which is not allowed`, d.Extension.ID)
		result.Code = -1
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

type detectable interface {
	API() string
	ClearEnv() bool
	RootDir() string
}

func runDetect(d detectable, platformDir, planPath, appDir string, bpEnv BuildEnv) DetectRun {
	out := &bytes.Buffer{}
	cmd := exec.Command(
		filepath.Join(d.RootDir(), "bin", "detect"),
		platformDir,
		planPath,
	) // #nosec G204
	cmd.Dir = appDir
	cmd.Stdout = out
	cmd.Stderr = out

	var err error
	if d.ClearEnv() {
		cmd.Env = bpEnv.List()
	} else {
		cmd.Env, err = bpEnv.WithPlatform(platformDir)
		if err != nil {
			return DetectRun{Code: -1, Err: err}
		}
	}
	cmd.Env = append(cmd.Env, EnvBuildpackDir+"="+d.RootDir())
	if api.MustParse(d.API()).AtLeast("0.8") {
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
