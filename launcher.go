package lifecycle

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/buildpack/packs"
)

const launcher = `
if compgen -G "$1/*/*/profile.d/*" > /dev/null; then
  for script in "$1"/*/*/profile.d/*; do
    [[ $script == $1/app/* ]] || [[ ! -f $script ]] && continue
    source "$script"
  done
fi

if [[ -f .profile ]]; then
  source .profile
fi

shift
exec bash -c "$@"
`

type Launcher struct {
	DefaultProcessType string
	DefaultLaunchDir   string
	DefaultAppDir      string
	Processes          []Process
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

	if err := l.Exec("/bin/bash", []string{
		"bash", "-c",
		launcher, executable,
		l.DefaultLaunchDir,
		startCommand,
	}, os.Environ()); err != nil {
		return packs.FailErrCode(err, packs.CodeFailedLaunch, "launch")
	}
	return nil
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
