package launch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func OSExecFunc(argv0 string, argv []string, envv []string) error {
	c := OSExecCmd(argv, envv)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func OSExecCmd(argv []string, envv []string) *exec.Cmd {
	var c *exec.Cmd
	arg0 := filepath.Base(argv[0])
	if arg0 == "cmd" || arg0 == "cmd.exe" {
		c = exec.Command(argv[0])
		c.SysProcAttr = &syscall.SysProcAttr{CmdLine: strings.Join(argv[1:], " ")}
	} else {
		c = exec.Command(argv[0], argv[1:]...)
	}
	c.Env = envv
	return c
}
