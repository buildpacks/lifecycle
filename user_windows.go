package lifecycle

import (
	"os/exec"
)

func asUser(cmd *exec.Cmd, uid, gid int) *exec.Cmd {
	return cmd
}

func ensureOwner(path string, uid, gid int) error {
	return nil
}

func recursiveEnsureOwner(path string, uid, gid int) error {
	return nil
}
