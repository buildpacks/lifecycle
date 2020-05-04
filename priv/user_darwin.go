package priv

import "os"

func EnsureOwner(uid, gid int, paths ...string) error {
	return nil
}

func IsPrivileged() bool {
	return os.Getuid() == 0
}

func RunAs(uid, gid int) error {
	return nil
}

func SetEnvironmentForUser(uid int) error {
	return nil
}
