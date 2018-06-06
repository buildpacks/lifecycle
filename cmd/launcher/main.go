package main

import (
	"os"

	"strings"

	"syscall"

	"github.com/sclevine/lifecycle"
	"github.com/sclevine/packs"
)

var startCommand string

const launcher = `
if compgen -G "$1/*/profile.d/*" > /dev/null; then
  for script in $1/*/profile.d/*; do
    [[ ! -f $script ]] && continue
    source "$script"
  done
fi
shift
exec bash -c "$@"
`

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
	if err := env.AddRootDir(lifecycle.DefaultLaunchDir); err != nil {
		return packs.FailErr(err, "modify env using", lifecycle.DefaultLaunchDir)
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
