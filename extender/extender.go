package extender

import (
	"os"
	"os/exec"
	"syscall"
)

type Opts struct {
	BaseImageRef string
	Kind         string

	Applier     DockerfileApplier
	Dockerfiles []Dockerfile
	IgnorePaths []string
}

type DockerfileApplier interface {
	ApplyBuild(dockerfiles []Dockerfile, image string, ignorePaths []string) error
	ApplyRun(dockerfiles []Dockerfile, image string, ignorePaths []string) error
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

func Extend(opts Opts) error {
	// TODO: pass logger in apply function maybe
	if opts.Kind == "build" {
		if err := opts.Applier.ApplyBuild(opts.Dockerfiles, opts.BaseImageRef, opts.IgnorePaths); err != nil {
			return err
		}
		return newBuildCmd().Run()
	}
	return opts.Applier.ApplyRun(opts.Dockerfiles, opts.BaseImageRef, opts.IgnorePaths)
}

func newBuildCmd() *exec.Cmd {
	// TODO: use the priv package to drop privileges and call lifecycle/cmd.Run(buildCmd)
	cmd := exec.Command("/cnb/lifecycle/builder", "-app", "/workspace", "-log-level", "debug") // TODO: pass app directory in opts
	cmd.Env = append(cmd.Env, "CNB_PLATFORM_API=0.8")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 1000, Gid: 1000}}
	return cmd
}
