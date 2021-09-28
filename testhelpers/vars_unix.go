// +build linux darwin

package testhelpers

import "golang.org/x/sys/unix"

func ExpectedUmask() int {
	expectedUmask := unix.Umask(0)
	defer unix.Umask(expectedUmask)
	return expectedUmask
}
