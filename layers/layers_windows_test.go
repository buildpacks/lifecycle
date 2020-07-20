package layers_test

import (
	"archive/tar"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	h "github.com/buildpacks/lifecycle/testhelpers"
)

func parentHeader(filePath string, fi os.FileInfo) *tar.Header {
	hdr := &tar.Header{
		Typeflag: tar.TypeDir,
	}
	hdr.Name = tarPath(filePath)
	return hdr
}

func tarPath(filePath string) string {
	return path.Join("Files", filepath.ToSlash(strings.TrimPrefix(
		filePath,
		filepath.VolumeName(filePath)+`\`,
	)))
}

func assertOSSpecificFields(t *testing.T, expected *tar.Header, hdr *tar.Header) {
	t.Helper()
	h.AssertEq(t, hdr.Format, tar.FormatPAX)
}

func assertOSSpecificEntries(t *testing.T, tr *tar.Reader) {
	for _, windowsEntry := range []string{"Files", "Hives"} {
		header, err := tr.Next()
		if err == io.EOF {
			t.Fatalf("missing expected archive entry '%s'", windowsEntry)
		}
		if header.Typeflag != tar.TypeDir {
			t.Fatalf("expected entry '%s' to have type %q, got %q", windowsEntry, header.Typeflag, tar.TypeDir)
		}
	}
}
