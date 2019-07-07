package lifecycle

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

type Launcher struct {
	DefaultProcessType string
	LayersDir          string
	AppDir             string
	Processes          []Process
	Buildpacks         []Buildpack
	Env                BuildEnv
	Exec               func(argv0 string, argv []string, envv []string) error
}

func (l *Launcher) Launch(executable, startCommand string) error {
	if err := l.env(); err != nil {
		return errors.Wrap(err, "modify env")
	}
	process, err := l.processFor(startCommand)
	if err != nil {
		return errors.Wrap(err, "determine start command")
	}
	if err := os.Chdir(l.AppDir); err != nil {
		return errors.Wrap(err, "change to app directory")
	}
	if process.Args != nil {
		binary, err := exec.LookPath(process.Command)
		if err != nil {
			return errors.Wrap(err, "path lookup")
		}

		if err := l.Exec(binary,
			append([]string{process.Command}, process.Args...),
			l.Env.List(),
		); err != nil {
			return errors.Wrap(err, "direct exec")
		}
		return nil
	}
	launcher, err := l.profileD()
	if err != nil {
		return errors.Wrap(err, "determine profile")
	}
	if err := l.Exec("/bin/bash", []string{
		"bash", "-c",
		launcher, executable,
		process.Command,
	}, l.Env.List()); err != nil {
		return errors.Wrap(err, "bash exec")
	}
	return nil
}

func (l *Launcher) env() error {
	appInfo, err := os.Stat(l.AppDir)
	if err != nil {
		return errors.Wrap(err, "find app directory")
	}
	return l.eachBuildpack(l.LayersDir, func(path string) error {
		bpInfo, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			} else {
				return errors.Wrap(err, "find buildpack directory")
			}
		}
		if os.SameFile(appInfo, bpInfo) {
			return nil
		}
		if err := eachDir(path, func(path string) error {
			return l.Env.AddRootDir(path)
		}); err != nil {
			return errors.Wrap(err, "add layer root")
		}
		if err := eachDir(path, func(path string) error {
			if err := l.Env.AddEnvDir(filepath.Join(path, "env")); err != nil {
				return err
			}
			return l.Env.AddEnvDir(filepath.Join(path, "env.launch"))
		}); err != nil {
			return errors.Wrap(err, "add layer env")
		}
		return nil
	})
}

func (l *Launcher) profileD() (string, error) {
	var out []string

	appendIfFile := func(path string) error {
		fi, err := os.Stat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			out = append(out, fmt.Sprintf(`source "%s"`, path))
		}
		return nil
	}
	layersDir, err := filepath.Abs(l.LayersDir)
	if err != nil {
		return "", err
	}
	for _, bp := range l.Buildpacks {
		scripts, err := filepath.Glob(filepath.Join(layersDir, bp.dir(), "*", "profile.d", "*"))
		if err != nil {
			return "", err
		}
		for _, script := range scripts {
			if err := appendIfFile(script); err != nil {
				return "", err
			}
		}
	}

	if err := appendIfFile(filepath.Join(l.AppDir, ".profile")); err != nil {
		return "", err
	}

	out = append(out, `exec bash -c "$@"`)
	return strings.Join(out, "\n"), nil
}

func (l *Launcher) processFor(cmd string) (Process, error) {
	if cmd == "" {
		if process, ok := l.findProcessType(l.DefaultProcessType); ok {
			return process, nil
		}

		return Process{}, fmt.Errorf("process type %s was not found", l.DefaultProcessType)
	}

	if process, ok := l.findProcessType(cmd); ok {
		return process, nil
	}

	return Process{Command: cmd}, nil
}

func (l *Launcher) findProcessType(kind string) (Process, bool) {
	for _, p := range l.Processes {
		if p.Type == kind {
			return p, true
		}
	}

	return Process{}, false
}

func eachDir(dir string, fn func(path string) error) error {
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
		if err := fn(filepath.Join(dir, f.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (l *Launcher) eachBuildpack(dir string, fn func(path string) error) error {
	for _, bp := range l.Buildpacks {
		if err := fn(filepath.Join(l.LayersDir, bp.dir())); err != nil {
			return err
		}
	}
	return nil
}
