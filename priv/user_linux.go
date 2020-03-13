// +build linux

package priv

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
)

/*
#cgo LDFLAGS: --static
#define _GNU_SOURCE
#include <unistd.h>
#include <errno.h>

static int
csetresuid(uid_t ruid, uid_t euid, uid_t suid) {
  int ec = setresuid(ruid, euid, suid);
  return (ec < 0) ? errno : 0;
}

static int
csetresgid(gid_t rgid, gid_t egid, gid_t sgid) {
  int ec = setresgid(rgid, egid, sgid);
  return (ec < 0) ? errno : 0;
}

*/
import "C"

// EnsureOwner recursively chowns a dir if it isn't owned by the given group
func EnsureOwner(uid, gid int, paths ...string) error {
	for _, p := range paths {
		fi, err := os.Stat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if stat, ok := fi.Sys().(*syscall.Stat_t); ok && stat.Gid == uint32(gid) {
			// if a dir has correct ownership, assume it's children do, for performance
			continue
		}
		if err := recursiveEnsureOwner(p, uid, gid); err != nil {
			return err
		}
	}
	return nil
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
	if err := setresgid(gid, gid, gid); err != nil {
		return err
	}
	if err := setresuid(uid, uid, uid); err != nil {
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

func setresgid(rgid, egid, sgid int) error {
	eno := C.csetresgid(C.gid_t(rgid), C.gid_t(egid), C.gid_t(sgid))
	if eno != 0 {
		return syscall.Errno(eno)
	}
	return nil
}

func setresuid(ruid, euid, suid int) error {
	eno := C.csetresuid(C.uid_t(ruid), C.uid_t(euid), C.uid_t(suid))
	if eno != 0 {
		return syscall.Errno(eno)
	}
	return nil
}
