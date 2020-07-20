// +build linux darwin

package layers_test

import (
	"archive/tar"
	"os"
	"syscall"
	"testing"
)

func parentHeader(filePath string, fi os.FileInfo) *tar.Header {
	hdr := &tar.Header{
		Name:     filePath,
		Typeflag: tar.TypeDir,
	}
	hdr.Name = filePath
	sys := fi.Sys().(*syscall.Stat_t)
	hdr.Uid = int(sys.Uid)
	hdr.Gid = int(sys.Gid)
	return hdr
}

func tarPath(filePath string) string {
	return filePath
}

func assertOSSpecificFields(t *testing.T, expected *tar.Header, hdr *tar.Header) {
	t.Helper()
	if hdr.Uid != expected.Uid {
		t.Fatalf("expected entry '%s' to have UID %d, got %d", expected.Name, expected.Uid, hdr.Uid)
	}
	if hdr.Gid != expected.Gid {
		t.Fatalf("expected entry '%s' to have GID %d, got %d", expected.Name, expected.Gid, hdr.Gid)
	}
}

func assertOSSpecificEntries(t *testing.T, tr *tar.Reader) {
	// unix layers have no OS specific entries
}
