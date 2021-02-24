//+build linux darwin

package launch

import (
	"os"
	"os/exec"
)

func setHandle(cmd *exec.Cmd, pw *os.File) error {
	cmd.ExtraFiles = []*os.File{pw}
	return nil
}
