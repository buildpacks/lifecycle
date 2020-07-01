package launch

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pkg/errors"
)

type ExecFunc func(argv0 string, argv []string, envv []string) error

type Launcher struct {
	DefaultProcessType string
	LayersDir          string
	AppDir             string
	Processes          []Process
	Buildpacks         []Buildpack
	Env                Env
	Exec               ExecFunc
	Setenv             func(string, string) error
}

func (l *Launcher) Launch(self string, cmd []string) error {
	process, err := l.processFor(cmd)
	if err != nil {
		return errors.Wrap(err, "determine start command")
	}
	if err := l.env(process); err != nil {
		return errors.Wrap(err, "modify env")
	}
	if err := os.Chdir(l.AppDir); err != nil {
		return errors.Wrap(err, "change to app directory")
	}
	if process.Direct {
		if err := l.Setenv("PATH", l.Env.Get("PATH")); err != nil {
			return errors.Wrap(err, "set path")
		}
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
	profileCmds, err := l.profileD(process)
	if err != nil {
		return errors.Wrap(err, "determine profile")
	}

	if runtime.GOOS == "windows" {
		var launcher string
		if len(profileCmds) > 0 {
			launcher = strings.Join(profileCmds, " && ") + " && "
		}
		if err := l.Exec("cmd",
			append(append([]string{"cmd", "/q", "/s", "/c"}, launcher, process.Command), process.Args...), l.Env.List(),
		); err != nil {
			return errors.Wrap(err, "cmd execute")
		}
		return nil
	}

	launcher := strings.Join(append(profileCmds, `exec bash -c "$@"`), "\n")
	if err := l.Exec("/bin/bash", append([]string{
		"bash", "-c",
		launcher, self, process.Command,
	}, process.Args...), l.Env.List()); err != nil {
		return errors.Wrap(err, "bash exec")
	}
	return nil
}

func (l *Launcher) env(process Process) error {
	appInfo, err := os.Stat(l.AppDir)
	if err != nil {
		return errors.Wrap(err, "find app directory")
	}
	return l.eachBuildpack(l.LayersDir, func(path string) error {
		bpInfo, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return errors.Wrap(err, "find buildpack directory")
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
			if err := l.Env.AddEnvDir(filepath.Join(path, "env.launch")); err != nil {
				return err
			}
			if process.Type == "" {
				return nil
			}
			return l.Env.AddEnvDir(filepath.Join(path, "env.launch", process.Type))
		}); err != nil {
			return errors.Wrap(err, "add layer env")
		}
		return nil
	})
}

func (l *Launcher) profileD(process Process) ([]string, error) {
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
			if runtime.GOOS == "windows" {
				out = append(out, fmt.Sprintf(`call %s`, path))
			} else {
				out = append(out, fmt.Sprintf(`source "%s"`, path))
			}
		}
		return nil
	}
	layersDir, err := filepath.Abs(l.LayersDir)
	if err != nil {
		return nil, err
	}
	for _, bp := range l.Buildpacks {
		fileGlob := "*"
		if runtime.GOOS == "windows" {
			fileGlob += ".bat"
		}
		globPaths := []string{filepath.Join(layersDir, EscapeID(bp.ID), "*", "profile.d", fileGlob)}
		if process.Type != "" {
			globPaths = append(globPaths, filepath.Join(layersDir, EscapeID(bp.ID), "*", "profile.d", process.Type, fileGlob))
		}
		var scripts []string
		for _, globPath := range globPaths {
			matches, err := filepath.Glob(globPath)
			if err != nil {
				return nil, err
			}
			scripts = append(scripts, matches...)
		}
		for _, script := range scripts {
			if err := appendIfFile(script); err != nil {
				return nil, err
			}
		}
	}

	profile := ".profile"
	if runtime.GOOS == "windows" {
		profile += ".bat"
	}
	if err := appendIfFile(filepath.Join(l.AppDir, profile)); err != nil {
		return nil, err
	}

	return out, nil
}

func (l *Launcher) processFor(cmd []string) (Process, error) {
	if len(cmd) == 0 {
		if process, ok := l.findProcessType(l.DefaultProcessType); ok {
			return process, nil
		}

		return Process{}, fmt.Errorf("process type %s was not found", l.DefaultProcessType)
	}

	if len(cmd) == 1 {
		if process, ok := l.findProcessType(cmd[0]); ok {
			return process, nil
		}
	}

	if len(cmd) > 1 && cmd[0] == "--" {
		return Process{Command: cmd[1], Args: cmd[2:], Direct: true}, nil
	}

	return Process{Command: cmd[0], Args: cmd[1:]}, nil
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
		if err := fn(filepath.Join(l.LayersDir, EscapeID(bp.ID))); err != nil {
			return err
		}
	}
	return nil
}
