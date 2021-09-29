package layers

import (
	"archive/tar"
	"io"
	"runtime"

	"github.com/buildpacks/lifecycle/archive"
)

// Extract extracts entries from r to the dest directory
// Contents of r should be an OCI layer.
// If dest is an empty string files with be extracted to `/` or `c:\` on unix and windows filesystems respectively.
// The umask must be unset before calling this function, to ensure that files have the correct file mode.
func Extract(r io.Reader, dest string, dirUmask int) error {
	tr := tarReader(r, dest)
	return archive.Extract(tr, dirUmask)
}

func tarReader(r io.Reader, dest string) archive.TarReader {
	tr := archive.NewNormalizingTarReader(tar.NewReader(r))
	if runtime.GOOS == "windows" {
		tr.ExcludePaths([]string{"Hives"})
		tr.Strip(`Files/`)
		if dest == "" {
			dest = `c:\`
		}
	}
	if dest == "" {
		dest = `/`
	}
	tr.PrependDir(dest)
	return tr
}
