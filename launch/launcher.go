package launch

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
)

var (
	LifecycleDir = filepath.Join(CNBDir, "lifecycle")
	ProcessDir   = filepath.Join(CNBDir, "process")
	LauncherPath = filepath.Join(LifecycleDir, "launcher"+exe)
)

type Launcher struct {
	AppDir             string
	Buildpacks         []Buildpack
	DefaultProcessType string
	Env                Env
	Exec               ExecFunc
	ExecD              ExecD
	Shell              Shell
	LayersDir          string
	PlatformAPI        *api.Version
	Processes          []Process
	Setenv             func(string, string) error
}

type ExecFunc func(argv0 string, argv []string, envv []string) error

type ExecD interface {
	ExecD(path string, env Env) error
}

type Env interface {
	AddEnvDir(envDir string) error
	AddRootDir(baseDir string) error
	Get(string) string
	List() []string
	Set(name, k string)
}

// Launch uses cmd to select a process and launches that process.
// For direct=false processes, self is used to set argv0 during profile script execution
func (l *Launcher) Launch(self string, cmd []string) error {
	process, err := l.ProcessFor(cmd)
	if err != nil {
		return errors.Wrap(err, "determine start command")
	}
	return l.LaunchProcess(self, process)
}

// LaunchProcess launches the provided process.
// For direct=false processes, self is used to set argv0 during profile script execution
func (l *Launcher) LaunchProcess(self string, process Process) error {
	if err := os.Chdir(l.AppDir); err != nil {
		return errors.Wrap(err, "change to app directory")
	}
	if err := l.doEnv(process); err != nil {
		return errors.Wrap(err, "modify env")
	}
	if err := l.doExecD(process); err != nil {
		return errors.Wrap(err, "exec.d")
	}

	if process.Direct {
		return l.launchDirect(process)
	}
	return l.launchWithShell(self, process)
}

func (l *Launcher) launchDirect(process Process) error {
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

func (l *Launcher) doEnv(process Process) error {
	return l.eachBuildpack(func(bpDir string) error {
		if err := eachLayer(bpDir, l.doLayerRoot()); err != nil {
			return errors.Wrap(err, "add layer root")
		}
		if err := eachLayer(bpDir, l.doLayerEnvFiles(process)); err != nil {
			return errors.Wrap(err, "add layer env")
		}
		return nil
	})
}

func (l *Launcher) doExecD(process Process) error {
	return l.eachBuildpack(func(path string) error {
		return eachLayer(path, l.doLayerExecD(process))
	}, supportsExecD)
}

func supportsExecD(bp Buildpack) bool {
	if bp.API == "" {
		return false
	}
	return api.MustParse(bp.API).Compare(api.MustParse("0.5")) >= 0
}

type action func(path string) error
type bpPredicate func(bp Buildpack) bool

func (l *Launcher) eachBuildpack(fn action, predicates ...bpPredicate) error {
	for _, bp := range l.Buildpacks {
		var skip bool
		for _, pred := range predicates {
			skip = skip || !pred(bp)
		}
		if skip {
			continue
		}

		dir := filepath.Join(l.LayersDir, EscapeID(bp.ID))
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return errors.Wrap(err, "find buildpack directory")
		}
		if err := fn(dir); err != nil {
			return err
		}
	}
	return nil
}

func (l *Launcher) doLayerRoot() action {
	return func(path string) error {
		return l.Env.AddRootDir(path)
	}
}

func (l *Launcher) doLayerEnvFiles(proc Process) action {
	return func(path string) error {
		if err := l.Env.AddEnvDir(filepath.Join(path, "env")); err != nil {
			return err
		}
		if err := l.Env.AddEnvDir(filepath.Join(path, "env.launch")); err != nil {
			return err
		}
		if proc.Type == "" {
			return nil
		}
		return l.Env.AddEnvDir(filepath.Join(path, "env.launch", proc.Type))
	}
}

func (l *Launcher) doLayerExecD(proc Process) action {
	return func(path string) error {
		if err := eachFile(filepath.Join(path, "exec.d"), func(path string) error {
			return l.ExecD.ExecD(path, l.Env)
		}); err != nil {
			return err
		}
		if proc.Type == "" {
			return nil
		}
		return eachFile(filepath.Join(path, "exec.d", proc.Type), func(path string) error {
			return l.ExecD.ExecD(path, l.Env)
		})
	}
}

func eachLayer(bpDir string, action action) error {
	return eachInDir(bpDir, action, func(fi os.FileInfo) bool {
		return fi.IsDir()
	})
}

func eachFile(dir string, action action) error {
	return eachInDir(dir, action, func(fi os.FileInfo) bool {
		return !fi.IsDir()
	})
}

func eachInDir(dir string, action action, pred func(fi os.FileInfo) bool) error {
	fis, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.Wrapf(err, "failed to list files in dir '%s'", dir)
	}
	for _, fi := range fis {
		if !pred(fi) {
			continue
		}
		if err := action(filepath.Join(dir, fi.Name())); err != nil {
			return err
		}
	}
	return nil
}
