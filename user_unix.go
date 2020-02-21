// +build linux darwin

package lifecycle

import (
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

var currentUser = os.Getuid()

// asUser sets HOME and non-root user credentials on command
func asUser(cmd *exec.Cmd, uid, gid int) (*exec.Cmd, error) {
	if currentUser != 0 {
		cmd.Env = append(cmd.Env, "HOME="+os.Getenv("HOME"))
		return cmd, nil
	}
	user, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return nil, err
	}
	cmd.Env = append(cmd.Env, "HOME="+user.HomeDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},
	}
	return cmd, nil
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
