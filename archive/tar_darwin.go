// +build darwin

package archive

import (
	"os/exec"
	"strconv"
	"strings"
)

func GetUmask() (int, error) {
	cmd := exec.Command("umask") // #nosec G204
	output, err := cmd.CombinedOutput()
	if err != nil {
		return -1, err
	}
	cleanedOutput := strings.Trim(string(output), "\n")
	current, err := strconv.ParseInt(cleanedOutput, 8, 64)
	if err != nil {
		return -1, err
	}
	return int(current), nil
}
