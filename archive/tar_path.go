package archive

import (
	"path/filepath"
	"strings"
)

// TarPath converts an OS path to a path suitable for TAR headers
func TarPath(path string) string {
	volumeName := filepath.VolumeName(path)
	path = strings.TrimPrefix(path, volumeName)
	return filepath.ToSlash(path)
}
