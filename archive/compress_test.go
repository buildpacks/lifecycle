package archive_test

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/archive"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestArchiveWrite(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "tar", testWrite, spec.Report(report.Terminal{}))
}

func testWrite(t *testing.T, when spec.G, it spec.S) {
	var tmpDir string

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "archive-write-test")
		h.AssertNil(t, err)
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(tmpDir))
	})

	when("#AddDirToArchive", func() {
		var (
			uid  = 1234
			gid  = 4567
			tw   *archive.NormalizingTarWriter
			file *os.File
		)

		it.Before(func() {
			var err error
			file, err = os.Create(filepath.Join(tmpDir, "tar_test-go.tar"))
			h.AssertNil(t, err)
			tw = &archive.NormalizingTarWriter{TarWriter: tar.NewWriter(file)}
			tw.WithUID(uid)
			tw.WithGID(gid)
			tw.WithModTime(archive.NormalizedModTime)
		})

		it.After(func() {
			file.Close()
		})

		for _, src := range []string{
			filepath.Join("testdata", "dir-to-tar"),
			filepath.Join("testdata", "dir-to-tar") + string(filepath.Separator),
			filepath.Join("testdata", "dir-to-tar") + string(filepath.Separator) + ".",
		} {
			src := src
			it(fmt.Sprintf("writes a tar with the src filesystem contents (%s)", src), func() {
				h.AssertNil(t, archive.AddDirToArchive(tw, src))
				h.AssertNil(t, file.Close())

				file, err := os.Open(file.Name())
				h.AssertNil(t, err)

				defer file.Close()
				tr := tar.NewReader(file)

				tarContains(t, "directories", func() {
					header, err := tr.Next()
					h.AssertNil(t, err)
					h.AssertEq(t, header.Name, "testdata/dir-to-tar")
					assertDirectory(t, header)
					assertModTimeNormalized(t, header)
				})

				tarContains(t, "directory symlinks", func() {
					header, err := tr.Next()
					h.AssertNil(t, err)

					h.AssertEq(t, header.Name, "testdata/dir-to-tar/dir-link")
					h.AssertEq(t, header.Uid, uid)
					h.AssertEq(t, header.Gid, gid)
					assertSymlink(t, header)
					h.AssertEq(t, header.Linkname, filepath.FromSlash("../excluded-dir"))
					assertModTimeNormalized(t, header)
				})

				tarContains(t, "file symlinks", func() {
					header, err := tr.Next()
					h.AssertNil(t, err)

					h.AssertEq(t, header.Name, "testdata/dir-to-tar/file-link")
					h.AssertEq(t, header.Uid, uid)
					h.AssertEq(t, header.Gid, gid)
					assertSymlink(t, header)
					h.AssertEq(t, header.Linkname, filepath.FromSlash("../excluded-dir/excluded-file"))
					assertModTimeNormalized(t, header)
				})

				tarContains(t, "regular files", func() {
					header, err := tr.Next()
					h.AssertNil(t, err)
					h.AssertEq(t, header.Name, "testdata/dir-to-tar/some-file.txt")

					fileContents := make([]byte, header.Size)
					_, err = tr.Read(fileContents)
					h.AssertSameInstance(t, err, io.EOF)
					h.AssertEq(t, string(fileContents), "some-content")
					h.AssertEq(t, header.Uid, uid)
					h.AssertEq(t, header.Gid, gid)
					assertModTimeNormalized(t, header)
				})

				tarContains(t, "subdir", func() {
					header, err := tr.Next()
					h.AssertNil(t, err)
					h.AssertEq(t, header.Name, "testdata/dir-to-tar/sub-dir")
					assertDirectory(t, header)
					assertModTimeNormalized(t, header)
				})

				tarContains(t, "children of subdir", func() {
					header, err := tr.Next()
					h.AssertNil(t, err)
					h.AssertEq(t, header.Name, "testdata/dir-to-tar/sub-dir/sub-file")
					assertModTimeNormalized(t, header)
				})
			})
		}
	})
}

func tarContains(t *testing.T, m string, r func()) {
	t.Helper()
	r()
}

func assertDirectory(t *testing.T, header *tar.Header) {
	t.Helper()
	if header.Typeflag != tar.TypeDir {
		t.Fatalf(`expected %s to be a directory`, header.Name)
	}
}

func assertSymlink(t *testing.T, header *tar.Header) {
	t.Helper()
	if header.Typeflag != tar.TypeSymlink {
		t.Fatalf(`expected %s to be a symlink`, header.Name)
	}
}

func assertModTimeNormalized(t *testing.T, header *tar.Header) {
	t.Helper()
	if !header.ModTime.Equal(time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)) {
		t.Fatalf(`expected %s time to be normalized, instead got: %s`, header.Name, header.ModTime.String())
	}
}
