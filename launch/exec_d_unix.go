//go:build linux || darwin || freebsd
// +build linux darwin freebsd

package launch

import (
	"os"
	"os/exec"
)

func setHandle(cmd *exec.Cmd, f *os.File) error {
	cmd.ExtraFiles = []*os.File{f}
	return nil
}
