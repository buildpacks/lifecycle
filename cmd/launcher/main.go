package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sclevine/packs"

	"github.com/sclevine/lifecycle"
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

var startCommand string

func main() {
	if len(os.Args) < 2 {
		packs.Exit(packs.FailCode(packs.CodeInvalidArgs, "parse start command"))
	}
	startCommand = strings.Join(os.Args[1:], " ")
	packs.Exit(launch())
}

func launch() error {
	env := &lifecycle.Env{
		Getenv:  os.Getenv,
		Setenv:  os.Setenv,
		Environ: os.Environ,
		Map:     lifecycle.POSIXLaunchEnv,
	}
	if err := eachDir(lifecycle.DefaultLaunchDir, func(bp string) error {
		if bp == "app" {
			return nil
		}
		bpPath := filepath.Join(lifecycle.DefaultLaunchDir, bp)
		return eachDir(bpPath, func(layer string) error {
			return env.AddRootDir(filepath.Join(bpPath, layer))
		})
	}); err != nil {
		return packs.FailErr(err, "modify env")
	}
	if err := os.Chdir(lifecycle.DefaultAppDir); err != nil {
		return packs.FailErr(err, "change directory to", lifecycle.DefaultAppDir)
	}
	if err := syscall.Exec("/bin/bash", []string{
		"bash", "-c",
		launcher, os.Args[0],
		lifecycle.DefaultLaunchDir,
		startCommand,
	}, os.Environ()); err != nil {
		return packs.FailErrCode(err, packs.CodeFailedLaunch, "launch")
	}
	return nil
}

func eachDir(dir string, fn func(file string) error) error {
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
