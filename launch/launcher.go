package launch

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/buildpacks/lifecycle/api"
)

const (
	ProfileDDirName = "profile.d"
	ExecDDirName    = "exec.d"
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
	if err := l.env(process); err != nil {
		return errors.Wrap(err, "modify env")
	}
	if err := l.execD(process); err != nil {
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

func (l *Launcher) findProcessType(kind string) (Process, bool) {
	for _, p := range l.Processes {
		if p.Type == kind {
			return p, true
		}
	}

	return Process{}, false
}

func (l *Launcher) env(process Process) error {
	appInfo, err := os.Stat(l.AppDir)
	if err != nil {
		return errors.Wrap(err, "find app directory")
	}
	return l.eachBuildpack(func(path string) error {
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

func (l *Launcher) execD(process Process) error {
	execDs, err := l.buildpackFiles(process, ExecDDirName, supportsExecD)
	if err != nil {
		return errors.Wrapf(err, "failed to find all exec.d executables in layers dir, '%s'", l.LayersDir)
	}
	for _, execD := range execDs {
		if err := l.ExecD.ExecD(execD, l.Env); err != nil {
			return err
		}
	}
	return nil
}

func supportsExecD(bp Buildpack) bool {
	if bp.API == "" {
		return false
	}
	return api.MustParse(bp.API).Compare(api.MustParse("0.5")) >= 0
}

func (l *Launcher) buildpackFiles(process Process, dirName string, predicates ...func(Buildpack) bool) ([]string, error) {
	var files []string

	appendIfFile := func(path string, fi os.FileInfo) {
		if !fi.IsDir() {
			files = append(files, path)
		}
	}

	appendFilesInDir := func(path string) error {
		fis, err := ioutil.ReadDir(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return errors.Wrapf(err, "failed to list files in dir '%s'", path)
		}

		for _, fi := range fis {
			appendIfFile(filepath.Join(path, fi.Name()), fi)
		}
		return nil
	}

	if err := l.eachBuildpack(func(path string) error {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		return eachDir(absPath, func(path string) error {
			if err := appendFilesInDir(filepath.Join(path, dirName)); err != nil {
				return err
			}
			if process.Type != "" {
				return appendFilesInDir(filepath.Join(path, dirName, process.Type))
			}
			return nil
		})
	}, predicates...); err != nil {
		return nil, err
	}
	return files, nil
}

func (l *Launcher) eachBuildpack(fn func(path string) error, predicates ...func(bp Buildpack) bool) error {
	for _, bp := range l.Buildpacks {
		var skip bool
		for _, pred := range predicates {
			skip = skip || !pred(bp)
		}
		if skip {
			continue
		}
		if err := fn(filepath.Join(l.LayersDir, EscapeID(bp.ID))); err != nil {
			return err
		}
	}
	return nil
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
