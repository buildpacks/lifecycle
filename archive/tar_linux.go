// +build linux

package archive

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

func GetUmask() (int, error) {
	path := "/proc/self/status"
	contents, err := os.ReadFile(path)
	if err != nil {
		return -1, err
	}
	lines := strings.Split(string(contents), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Umask:") {
			parts := strings.Split(line, ":")
			if len(parts) != 2 {
				return -1, errors.New("failed to get umask")
			}
			current, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 8, 64)
			if err != nil {
				return -1, err
			}
			return int(current), nil
		}
	}
	return -1, errors.New("failed to get umask")
}
