// +build linux

package priv

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// EnsureOwner recursively chowns a dir if it isn't writable
func EnsureOwner(uid, gid int, paths ...string) error {
	for _, p := range paths {
		fi, err := os.Stat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if stat, ok := fi.Sys().(*syscall.Stat_t); ok && canWrite(uid, gid, stat) {
			// if a dir has correct ownership, assume it's children do, for performance
			continue
		}
		if err := recursiveEnsureOwner(p, uid, gid); err != nil {
			return err
		}
	}
	return nil
}

const (
	worldWrite uint32 = 0002
	groupWrite uint32 = 0020
)

func canWrite(uid, gid int, stat *syscall.Stat_t) bool {
	if stat.Uid == uint32(uid) {
		// assume owner has write permission
		return true
	}
	if stat.Gid == uint32(gid) && stat.Mode&groupWrite != 0 {
		return true
	}
	if stat.Mode&worldWrite != 0 {
		return true
	}
	return false
}

func IsPrivileged() bool {
	return os.Getuid() == 0
}

func recursiveEnsureOwner(path string, uid, gid int) error {
	if err := os.Chown(path, uid, gid); err != nil {
		return err
	}
	fis, err := ioutil.ReadDir(path)
	if err != nil {
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

// RunAs sets the user ID and group ID of the calling process.
func RunAs(uid, gid int) error {
	if uid == os.Getuid() && gid == os.Getgid() {
		return nil
	}

	if err := syscall.Setresgid(gid, gid, gid); err != nil {
		return err
	}
	if err := syscall.Setresuid(uid, uid, uid); err != nil {
		return err
	}

	return nil
}

func SetEnvironmentForUser(uid int) error {
	user, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return err
	}
	if err := os.Setenv("HOME", user.HomeDir); err != nil {
		return err
	}
	if err := os.Setenv("USER", user.Name); err != nil {
		return err
	}
	return nil
}
