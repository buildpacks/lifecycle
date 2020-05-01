package priv

func EnsureOwner(uid, gid int, paths ...string) error {
	return nil
}

func IsPrivileged() bool {
	return false
}

func RunAs(uid, gid int, withUserLookup bool) error {
	return nil
}
