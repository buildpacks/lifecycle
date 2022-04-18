package extender

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/pkg/errors"
)

func Extend(opts Opts, logger Logger) error {
	if opts.Kind == "build" {
		if err := opts.Applier.ApplyBuild(opts.Dockerfiles, opts.BaseImageRef, opts.TargetImageRef, opts.IgnorePaths, logger); err != nil {
			return errors.Wrap(err, "applying dockerfiles")
		}
		extendedEnv, err := getImageConfigEnv(opts.TargetImageRef)
		if err != nil {
			return errors.Wrap(err, "getting extended environment")
		}
		return newBuildCmd(opts.BuilderOpts, extendedEnv).Run()
	}
	return opts.Applier.ApplyRun(opts.Dockerfiles, opts.BaseImageRef, opts.TargetImageRef, opts.IgnorePaths, logger)
}

func getImageConfigEnv(imageName string) ([]string, error) {
	ref, err := name.ParseReference(imageName)
	if err != nil {
		return nil, errors.Wrap(err, "parsing reference")
	}
	image, err := remote.Image(ref)
	if err != nil {
		return nil, errors.Wrap(err, "getting image")
	}
	config, err := image.ConfigFile()
	if err != nil || config == nil {
		return nil, errors.Wrap(err, "getting image config")
	}
	return config.Config.Env, nil
}

type Opts struct {
	BaseImageRef   string
	TargetImageRef string
	Kind           string

	Applier     DockerfileApplier
	Dockerfiles []Dockerfile
	IgnorePaths []string // TODO: should this go in kaniko constructor?

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

func newBuildCmd(opts BuilderOpts, env []string) *exec.Cmd {
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
	cmd.Env = append(env, "CNB_PLATFORM_API=0.8")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 1000, Gid: 1000}}
	return cmd
}
