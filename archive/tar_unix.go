// +build linux darwin

package archive

import (
	"archive/tar"
	"os"

	"golang.org/x/sys/unix"
)

func SetUmask(new int) (old int) {
	return unix.Umask(new)
}

func createSymlink(hdr *tar.Header) error {
	return os.Symlink(hdr.Linkname, hdr.Name)
}

func addSysAttributes(hdr *tar.Header, fi os.FileInfo) {
}
