package layers

import (
	"archive/tar"
	"io"
	"runtime"

	"github.com/buildpacks/lifecycle/archive"
)

func Extract(r io.Reader, dest string) error {
	tr := tarReader(r, dest)
	return archive.Extract(tr)
}

func tarReader(r io.Reader, dest string) archive.TarReader {
	tr := archive.NewNormalizingTarReader(tar.NewReader(r))
	tr.Strip("Hives")
	tr.Strip("Files")
	if runtime.GOOS == "windows" {
		tr.ExcludePaths([]string{"Hives"})
		tr.Strip(`Files`)
		tr.FromSlash()
		if dest == "" {
			dest = "c:"
		}
	}
	tr.PrependDir(dest)
	return tr
}
