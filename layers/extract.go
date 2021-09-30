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
// The umask must be unset before calling this function on unix, to ensure that files have the correct file mode. SetUmask can be used to set and unset the umask.
// The provided umask will be applied to new directories that are created as parent directories of files in the tar, that do not themselves have headers in the tar.
func Extract(r io.Reader, dest string, procUmask int) error {
	tr := tarReader(r, dest)
	return archive.Extract(tr, procUmask)
}

func SetUmask(newMask int) (oldMask int) {
	return archive.SetUmask(newMask)
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
