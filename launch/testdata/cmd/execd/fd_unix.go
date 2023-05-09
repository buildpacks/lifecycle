//go:build linux || darwin || freebsd
// +build linux darwin freebsd

package main

import "os"

func outputFile() (*os.File, error) {
	return os.NewFile(3, "outputFile"), nil
}
