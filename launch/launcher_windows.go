package launch

import (
	"os"
	"os/exec"
)

const (
	CNBDir = `c:\cnb`
	exe    = ".exe"
)

func OSExecFunc(argv0 string, argv []string, envv []string) error {
	c := exec.Command(argv[0], argv[1:]...)
	c.Env = envv
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
