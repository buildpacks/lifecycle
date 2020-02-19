// +build linux darwin

package lifecycle

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

var currentUser = os.Getuid()

func asUser(cmd *exec.Cmd, uid, gid int) *exec.Cmd {
	if currentUser != 0 {
		return cmd
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},
	}
	return cmd
}

func ensureOwner(path string, uid, gid int) error {
	if currentUser != 0 {
		return nil
	}
	return os.Lchown(path, uid, gid)
}

func recursiveEnsureOwner(path string, uid, gid int) error {
	if currentUser != 0 {
		return nil
	}
	fis, err := ioutil.ReadDir(path)
	if err != nil {
		return err
	}
	if err := os.Chown(path, uid, gid); err != nil {
		return err
	}
	for _, fi := range fis {
		filePath := filepath.Join(path, fi.Name())
		if fi.IsDir() {
			if err := recursiveEnsureOwner(filePath, uid, gid); err != nil {
				return err
			}
		} else {
			if err := os.Lchown(filePath, uid, gid); err != nil {
				return err
			}
		}
	}
	return nil
}
