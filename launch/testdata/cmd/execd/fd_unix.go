//go:build linux || darwin
// +build linux darwin

package main

import "os"

func outputFile() (*os.File, error) {
	return os.NewFile(3, "outputFile"), nil
}
