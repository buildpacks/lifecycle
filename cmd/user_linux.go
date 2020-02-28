// +build linux

package cmd

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

func EnsureOwner(uid, gid int, paths ...string) error {
	for _, p := range paths {
		fi, err := os.Stat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if stat, ok := fi.Sys().(*syscall.Stat_t); ok && stat.Uid == uint32(uid) && stat.Gid == uint32(gid) {
			// if a dir has correct ownership, assume it's children do, for performance
			continue
		}
		if err := recursiveEnsureOwner(p, uid, gid); err != nil {
			return err
		}
	}
	return nil
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

func RunAs(uid, gid int) error {
	if uid == os.Getuid() && gid == os.Getgid() {
		return nil
	}
	user, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return err
	}

	// temporarily reduce to one thread b/c setres{gid,uid} works per thread on linux
	mxp := runtime.GOMAXPROCS(1)
	runtime.LockOSThread()
	if err := unix.Setresgid(gid, gid, gid); err != nil {
		return err
	}
	if err := unix.Setresuid(uid, uid, uid); err != nil {
		return err
	}
	_ = runtime.GOMAXPROCS(mxp)

	if err := os.Setenv("HOME", user.HomeDir); err != nil {
		return err
	}
	if err = os.Setenv("USER", user.Name); err != nil {
		return err
	}
	if _, ok := os.LookupEnv("DOCKER_CONFIG"); ok {
		return nil
	}
	// ggcr sets default docker config during init, fix for user
	if err := os.Setenv("DOCKER_CONFIG", filepath.Join(user.HomeDir, ".docker")); err != nil {
		return err
	}
	return nil
}
