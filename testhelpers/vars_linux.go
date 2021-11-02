//go:build linux
// +build linux

package testhelpers

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
)

func GetUmask(t *testing.T) int {
	path := "/proc/self/status"
	contents, err := os.ReadFile(path)
	AssertNil(t, err)
	lines := strings.Split(string(contents), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Umask:") {
			parts := strings.Split(line, ":")
			AssertEq(t, len(parts), 2)
			current, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 8, 64)
			AssertNil(t, err)
			return int(current)
		}
	}
	AssertNil(t, errors.New("failed to get umask"))
	return -1
}
