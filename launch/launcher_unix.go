//+build linux darwin

package launch

import "syscall"

const (
	CNBDir = `/cnb`
	exe    = ""
)

var OSExecFunc = syscall.Exec
