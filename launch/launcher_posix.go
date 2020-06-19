//+build linux darwin

package launch

import "syscall"

var OSExecFunc = syscall.Exec
