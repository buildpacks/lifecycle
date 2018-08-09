package lifecycle

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildpack/packs"
)

type Launcher struct {
	DefaultProcessType string
	DefaultLaunchDir   string
	DefaultAppDir      string
	Processes          []Process
	Buildpacks         []string
	Exec               func(argv0 string, argv []string, envv []string) error
}

func (l *Launcher) Launch(executable, startCommand string) error {
	env := &Env{
		Getenv:  os.Getenv,
		Setenv:  os.Setenv,
		Environ: os.Environ,
		Map:     POSIXLaunchEnv,
	}
	if err := l.eachDir(l.DefaultLaunchDir, func(bp string) error {
		if bp == "app" {
			return nil
		}
		bpPath := filepath.Join(l.DefaultLaunchDir, bp)
		return l.eachDir(bpPath, func(layer string) error {
			return env.AddRootDir(filepath.Join(bpPath, layer))
		})
	}); err != nil {
		return packs.FailErr(err, "modify env")
	}
	if err := os.Chdir(l.DefaultAppDir); err != nil {
		return packs.FailErr(err, "change directory to", l.DefaultAppDir)
	}

	startCommand, err := l.processFor(startCommand)
	if err != nil {
		return packs.FailErr(err, "determine start command")
	}

	launcher, err := l.profiled()
	if err != nil {
		return packs.FailErr(err, "determine profile")
	}

	if err := l.Exec("/bin/bash", []string{
		"bash", "-c",
		launcher, executable,
		startCommand,
	}, os.Environ()); err != nil {
		return packs.FailErrCode(err, packs.CodeFailedLaunch, "launch")
	}
	return nil
}

func (l *Launcher) profiled() (string, error) {
	var script []string

	appendIfFile := func(path string) error {
		fi, err := os.Stat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			script = append(script, fmt.Sprintf(`source "%s"`, path))
		}
		return nil
	}

	for _, bp := range l.Buildpacks {
		pdscripts, err := filepath.Glob(filepath.Join(l.DefaultLaunchDir, bp, "*", "profile.d", "*"))
		if err != nil {
			return "", err
		}
		for _, pdscript := range pdscripts {
			if err := appendIfFile(pdscript); err != nil {
				return "", err
			}
		}
	}

	if err := appendIfFile(filepath.Join(l.DefaultAppDir, ".profile")); err != nil {
		return "", err
	}

	script = append(script, `exec bash -c "$@"`)
	return strings.Join(script, "\n"), nil
}

func (l *Launcher) processFor(cmd string) (string, error) {
	if cmd == "" {
		if process, ok := l.findProcessType(l.DefaultProcessType); ok {
			return process, nil
		}

		return "", fmt.Errorf("process type %s was not available", l.DefaultProcessType)
	}

	if process, ok := l.findProcessType(cmd); ok {
		return process, nil
	}

	return cmd, nil
}

func (l *Launcher) findProcessType(kind string) (string, bool) {
	for _, p := range l.Processes {
		if p.Type == kind {
			return p.Command, true
		}
	}

	return "", false
}

func (*Launcher) eachDir(dir string, fn func(file string) error) error {
	files, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		if err := fn(f.Name()); err != nil {
			return err
		}
	}
	return nil
}
