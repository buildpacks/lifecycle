package archive_test

import (
	"archive/tar"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle/archive"
	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestTar(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "tar", testTar, spec.Report(report.Terminal{}))
}

func testTar(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir, src, tarFile string
		file                 *os.File
		uid                  = 1234
		gid                  = 2345
	)

	when("#ReadTarArchive", func() {

		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "extract-tar-test")

			if err != nil {
				t.Fatalf("failed to create tmp dir %s: %s", tmpDir, err)
			}
			h.AssertNil(t, err)
		})

		it.After(func() {
			err := os.RemoveAll(tmpDir)
			h.AssertNil(t, err)
		})

		it("extracts a tar file", func() {

			var pathModes = []archive.PathMode{
				{"root", os.ModeDir + 0755},
				{"root/readonly", os.ModeDir + 0500},
				{"root/standarddir", os.ModeDir + 0755},
				{"root/standarddir/somefile", 0644},
				{"root/readonly/readonlysub/somefile", 0444},
				{"root/readonly/readonlysub", os.ModeDir + 0500},
			}

			src = filepath.Join("testdata", "tar-to-dir", "some-layer.tar")
			file, err := os.Open(src)
			h.AssertNil(t, err)
			h.AssertNil(t, archive.Untar(file, tmpDir))

			for _, pathMode := range pathModes {
				file := filepath.Join(tmpDir, pathMode.Path)
				fileInfo, err := os.Stat(file)
				h.AssertNil(t, err)
				h.AssertEq(t, fileInfo.Mode(), pathMode.Mode)
				// Make all files os.ModePerm where they can all be cleaned up.
				if err := os.Chmod(file, os.ModePerm); err != nil {
					h.AssertNil(t, err)
				}
			}
		})

	})

	when("#WriteTarArchive", func() {
		it.Before(func() {
			var err error
			tmpDir, err = ioutil.TempDir("", "create-tar-test")
			if err != nil {
				t.Fatalf("failed to create tmp dir %s: %s", tmpDir, err)
			}

			tarFile = filepath.Join(tmpDir, "tar_test-go.tar")

			file, err = os.Create(tarFile)
			h.AssertNil(t, err)
		})

		it.After(func() {
			err := os.RemoveAll(tmpDir)
			h.AssertNil(t, err)

			err = os.RemoveAll(tarFile)
			h.AssertNil(t, err)
		})

		it("writes a tar with the src filesystem contents", func() {
			src = filepath.Join("testdata", "dir-to-tar")

			h.AssertNil(t, archive.WriteTarArchive(file, src, uid, gid))
			h.AssertNil(t, file.Close())

			file, err := os.Open(tarFile)
			h.AssertNil(t, err)

			defer file.Close()
			tr := tar.NewReader(file)

			tarContains(t, "directories", func() {
				header, err := tr.Next()
				h.AssertNil(t, err)
				h.AssertEq(t, header.Name, "testdata")
				assertModTimeNormalized(t, header)

				header, err = tr.Next()
				h.AssertNil(t, err)
				h.AssertEq(t, header.Name, "testdata/dir-to-tar")
				assertModTimeNormalized(t, header)
			})

			tarContains(t, "regular files", func() {
				header, err := tr.Next()
				h.AssertNil(t, err)
				h.AssertEq(t, header.Name, "testdata/dir-to-tar/some-file.txt")

				fileContents := make([]byte, header.Size, header.Size)
				tr.Read(fileContents)
				h.AssertEq(t, string(fileContents), "some-content")
				h.AssertEq(t, header.Uid, uid)
				h.AssertEq(t, header.Gid, gid)
				assertModTimeNormalized(t, header)
			})

			tarContains(t, "sub directories", func() {
				header, err := tr.Next()
				h.AssertNil(t, err)
				h.AssertEq(t, header.Name, "testdata/dir-to-tar/sub-dir")
				assertModTimeNormalized(t, header)
			})

			tarContains(t, "symlinks", func() {
				header, err := tr.Next()
				h.AssertNil(t, err)

				h.AssertEq(t, header.Name, "testdata/dir-to-tar/sub-dir/link-file")
				h.AssertEq(t, header.Uid, uid)
				h.AssertEq(t, header.Gid, gid)
				h.AssertEq(t, header.Linkname, "../some-file.txt")
				assertModTimeNormalized(t, header)
			})
		})

		when("a absolute path is given", func() {
			it("has working test helpers", func() {
				h.AssertEq(t, allParentDirectories("/some/absolute/directory"), []string{"/some", "/some/absolute"})
			})

			it("writes headers for all parent directories if an absolute path is given", func() {
				absoluteFilePath, err := filepath.Abs(filepath.Join("testdata", "dir-to-tar"))
				h.AssertNil(t, err)

				h.AssertNil(t, archive.WriteTarArchive(file, absoluteFilePath, uid, gid))
				h.AssertNil(t, file.Close())

				file, err = os.Open(tarFile)
				h.AssertNil(t, err)

				defer file.Close()
				tr := tar.NewReader(file)

				for _, expectedDir := range allParentDirectories(absoluteFilePath) {
					header, err := tr.Next()
					h.AssertNil(t, err)

					h.AssertEq(t, header.Name, expectedDir)

					assertDirectory(t, header)
					assertModTimeNormalized(t, header)
				}
			})
		})

		when("a relative path is given", func() {
			it("has working test helpers", func() {
				h.AssertEq(t, allParentDirectories("some/relative/path"), []string{"some", "some/relative"})
			})

			it("writes headers for all parent directories", func() {
				relativePath := filepath.Join("testdata", "dir-to-tar", "sub-dir")

				h.AssertNil(t, archive.WriteTarArchive(file, relativePath, uid, gid))
				h.AssertNil(t, file.Close())

				file, err := os.Open(tarFile)
				h.AssertNil(t, err)

				defer file.Close()
				tr := tar.NewReader(file)

				for _, expectedDir := range allParentDirectories(relativePath) {
					header, err := tr.Next()
					h.AssertNil(t, err)

					h.AssertEq(t, header.Name, expectedDir)

					assertDirectory(t, header)
					assertModTimeNormalized(t, header)

				}
			})
		})

		it("writes parent directories with the existing filesystem permissions", func() {
			tmpDir, err := ioutil.TempDir("", "tar-permissions-test")
			h.AssertNil(t, err)
			defer os.RemoveAll(tmpDir)

			src := filepath.Join(tmpDir, "parent-directory", "tar-directory")
			err = os.MkdirAll(src, 0764)
			h.AssertNil(t, err)

			h.AssertNil(t, archive.WriteTarArchive(file, src, uid, gid))
			h.AssertNil(t, file.Close())

			file, err = os.Open(tarFile)
			h.AssertNil(t, err)

			defer file.Close()
			tr := tar.NewReader(file)

			for _, expectedDir := range allParentDirectories(src) {
				header, err := tr.Next()
				h.AssertNil(t, err)
				h.AssertEq(t, header.Name, expectedDir)

				assertDirectory(t, header)

				localDir, err := os.Stat(expectedDir)
				h.AssertNil(t, err)

				assertPermissions(t, header, localDir.Mode().Perm())
			}
		})
	})
}

func tarContains(t *testing.T, m string, r func()) {
	t.Helper()
	t.Log(m)
	r()
}

func assertPermissions(t *testing.T, header *tar.Header, expectedMode os.FileMode) {
	t.Helper()
	if header.FileInfo().Mode().Perm() != expectedMode {
		t.Fatalf(`expectedMode %s to have permissions %o instead %o`, header.Name, expectedMode, header.Mode)
	}
}

func assertDirectory(t *testing.T, header *tar.Header) {
	t.Helper()
	if header.Typeflag != tar.TypeDir {
		t.Fatalf(`expected %s to be a directory`, header.Name)
	}
}

func assertModTimeNormalized(t *testing.T, header *tar.Header) {
	t.Helper()
	if !header.ModTime.Equal(time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)) {
		t.Fatalf(`expected %s time to be normalized, instead got: %s`, header.Name, header.ModTime.String())
	}
}

func allParentDirectories(directory string) []string {
	parent := filepath.Dir(directory)
	if parent == "." || parent == "/" {
		return []string{}
	} else {
		return append(allParentDirectories(parent), parent)
	}
}
