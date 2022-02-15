package extender

import (
	"os"
	"os/exec"
	"syscall"
)

func Extend(opts Opts, logger Logger) error {
	if opts.Kind == "build" {
		if err := opts.Applier.ApplyBuild(opts.Dockerfiles, opts.BaseImageRef, opts.TargetImageRef, opts.IgnorePaths, logger); err != nil {
			return err
		}
		return newBuildCmd(opts.BuilderOpts).Run()
	}
	return opts.Applier.ApplyRun(opts.Dockerfiles, opts.BaseImageRef, opts.TargetImageRef, opts.IgnorePaths, logger)
}

type Opts struct {
	BaseImageRef   string
	TargetImageRef string
	Kind           string

	Applier     DockerfileApplier
	Dockerfiles []Dockerfile
	IgnorePaths []string

	BuilderOpts
}

type BuilderOpts struct {
	AppDir        string
	BuildpacksDir string
	GroupPath     string
	LayersDir     string
	LogLevel      string
	PlanPath      string
	PlatformDir   string
}

type DockerfileApplier interface {
	ApplyBuild(dockerfiles []Dockerfile, baseImageRef, targetImageRef string, ignorePaths []string, logger Logger) error
	ApplyRun(dockerfiles []Dockerfile, baseImageRef string, targetImageRef string, ignorePaths []string, logger Logger) error
}

type Dockerfile struct {
	ExtensionID string          `toml:"extension_id"`
	Path        string          `toml:"path"`
	Type        string          `toml:"type"`
	Args        []DockerfileArg `toml:"args"`
}

type DockerfileArg struct {
	Key   string `toml:"name"`
	Value string `toml:"value"`
}

type Logger interface {
	Debug(msg string)
	Debugf(fmt string, v ...interface{})

	Info(msg string)
	Infof(fmt string, v ...interface{})

	Warn(msg string)
	Warnf(fmt string, v ...interface{})

	Error(msg string)
	Errorf(fmt string, v ...interface{})
}

func newBuildCmd(opts BuilderOpts) *exec.Cmd {
	// TODO: use the priv package to drop privileges and call lifecycle/cmd.Run(buildCmd)
	cmd := exec.Command(
		"/cnb/lifecycle/builder",
		"-app", opts.AppDir,
		"-buildpacks", opts.BuildpacksDir,
		"-group", opts.GroupPath,
		"-layers", opts.LayersDir,
		"-log-level", opts.LogLevel,
		"-plan", opts.PlanPath,
		"-platform", opts.PlatformDir,
	)
	cmd.Env = append(cmd.Env, "CNB_PLATFORM_API=0.8")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 1000, Gid: 1000}}
	return cmd
}
