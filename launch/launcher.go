package launch

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
	Env                Env
	Exec               func(argv0 string, argv []string, envv []string) error
	Setenv             func(string, string) error
}

func (l *Launcher) Launch(self string, cmd []string) error {
	if err := l.env(); err != nil {
		return errors.Wrap(err, "modify env")
	}
	process, err := l.processFor(cmd)
	if err != nil {
		return errors.Wrap(err, "determine start command")
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
	launchScript, err := l.profileD(process)
	if err != nil {
		return errors.Wrap(err, "determine profile")
	}
	if err := l.Exec("/bin/bash", append([]string{
		"bash", "-c",
		launchScript, self, process.Command,
	}, process.Args...), l.Env.List()); err != nil {
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
			return l.Env.AddEnvDir(filepath.Join(path, "env.launch"))
		}); err != nil {
			return errors.Wrap(err, "add layer env")
		}
		return nil
	})
}

func (l *Launcher) profileD(process Process) (string, error) {
	var out []string

	out = append(out, fmt.Sprintf("set -x"))
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
		scripts, err := filepath.Glob(filepath.Join(layersDir, EscapeID(bp.ID), "*", "profile.d", "*"))
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

	script := l.executionScript(process)
	fmt.Println(script)
	out = append(out, script)
	return strings.Join(out, "\n"), nil
}
func (l *Launcher) executionScript(process Process) string {
	if len(process.Args) == 0 {
		return `exec bash -c "$@"`
	}
	commandScript := `$(eval echo \"$0\")`
	for i := range process.Args {
		commandScript += fmt.Sprintf(` $(eval echo \"$%d\")`, 1+i)
	}
	return fmt.Sprintf(`exec bash -c '%s' "${@:1}"`, commandScript)
}

func (l *Launcher) processFor(cmd []string) (Process, error) {
	if l.DefaultProcessType == "override" {
		return userProvidedProcess(cmd)
	}
	process, err := l.findProcessType(l.DefaultProcessType)
	if err != nil {
		return Process{}, err
	}
	process.Args = append(process.Args, cmd...)
	return process, nil
}

func userProvidedProcess(cmd []string) (Process, error) {
	if len(cmd) == 0 {
		return Process{}, errors.New("process type was 'override' but no command was provided")
	}
	if len(cmd) > 1 && cmd[0] == "--" {
		return Process{Command: cmd[1], Args: cmd[2:], Direct: true}, nil
	}

	return Process{Command: cmd[0], Args: cmd[1:]}, nil
}

func (l *Launcher) findProcessType(processType string) (Process, error) {
	for _, p := range l.Processes {
		if p.Type == processType {
			return p, nil
		}
	}
	return Process{}, fmt.Errorf("process type %s was not found", processType)
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
