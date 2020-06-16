package archive

import (
	"path/filepath"
	"runtime"
	"strings"
)

// TarPath converts an OS path to a path suitable for TAR headers
func TarPath(path string) string {
	volumeName := filepath.VolumeName(path)
	path = strings.TrimPrefix(path, volumeName)
	return filepath.ToSlash(path)
}

// The windows container image filesystem contains special directories
// that are omitted when working with an already running container
func cleanImageLayerPath(path string) string {
	if runtime.GOOS != "windows" {
		return path
	}

	volumeName := filepath.VolumeName(path)

	for _, dirName := range []string{`\Files`, `\Hives`} {
		prefix := volumeName + dirName

		if strings.HasPrefix(path, prefix) {
			path = strings.TrimPrefix(path, prefix)

			if path == "" {
				path = `\`
			}

			path = filepath.Join(volumeName, path)
		}
	}

	return path
}
