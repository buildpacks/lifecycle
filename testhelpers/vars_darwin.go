//go:build darwin
// +build darwin

package testhelpers

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

func GetUmask(t *testing.T) int {
	cmd := exec.Command("umask") // #nosec G204
	output, err := cmd.CombinedOutput()
	AssertNil(t, err)
	cleanedOutput := strings.Trim(string(output), "\n")
	current, err := strconv.ParseInt(cleanedOutput, 8, 64)
	AssertNil(t, err)
	return int(current)
}
