//go:build linux || darwin

package priv

import (
	"net/url"
	"os"
	"syscall"
)

// shouldConnectSock returns true if the docker host is a root owned unix domain socket
func shouldConnectSock(host *url.URL) bool {
	if host.Scheme != "unix" {
		return false
	}
	fi, err := os.Stat(host.Path)
	if err != nil {
		return false
	}
	if stat, ok := fi.Sys().(*syscall.Stat_t); ok && stat.Uid == 0 {
		return true
	}
	return false
}
