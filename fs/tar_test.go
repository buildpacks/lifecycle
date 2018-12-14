package fs_test

import (
	"archive/tar"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle/fs"
	h "github.com/buildpack/lifecycle/testhelpers"
)

func TestFS(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "fs", testFS, spec.Report(report.Terminal{}))
}

func testFS(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir, src string
		fs          fs.FS
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "create-tar-test")
		h.AssertNil(t, err)
		src = filepath.Join("testdata", "dir-to-tar")
	})

	it.After(func() {
		err := os.RemoveAll(tmpDir)
		h.AssertNil(t, err)
	})

	it("writes a tar to the dest dir", func() {
		tarFile := filepath.Join(tmpDir, "some.tar")
		err := fs.CreateTarFile(tarFile, src, "/dir-in-archive", 1234, 2345)
		h.AssertNil(t, err)
		file, err := os.Open(tarFile)
		h.AssertNil(t, err)

		defer file.Close()
		tr := tar.NewReader(file)

		t.Log("handles directories")
		header, err := tr.Next()
		h.AssertNil(t, err)
		h.AssertEq(t, header.Name, "/dir-in-archive")

		t.Log("handles regular files")
		header, err = tr.Next()
		h.AssertNil(t, err)
		h.AssertEq(t, header.Name, "/dir-in-archive/some-file.txt")

		fileContents := make([]byte, header.Size, header.Size)
		tr.Read(fileContents)
		h.AssertEq(t, string(fileContents), "some-content")
		h.AssertEq(t, header.Uid, 1234)
		h.AssertEq(t, header.Gid, 2345)

		if runtime.GOOS != "windows" {
			t.Log("handles sub dir")
			_, err = tr.Next()
			h.AssertNil(t, err)

			t.Log("handles symlinks")
			header, err = tr.Next()
			h.AssertNil(t, err)

			h.AssertEq(t, header.Name, "/dir-in-archive/sub-dir/link-file")
			h.AssertEq(t, header.Uid, 1234)
			h.AssertEq(t, header.Gid, 2345)
			h.AssertEq(t, header.Linkname, "../some-file.txt")
		}
	})

	it("writes parent directory headers with the expected permissions", func() {
		tarFile := filepath.Join(tmpDir, "some.tar")
		err := fs.CreateTarFile(tarFile, src, "/expectedDir-in-archive/with/nested/path", 1234, 2345)
		h.AssertNil(t, err)

		file, err := os.Open(tarFile)
		h.AssertNil(t, err)
		defer file.Close()
		tr := tar.NewReader(file)

		t.Log("handles directories")

		for _, expectedDir := range []string{"/expectedDir-in-archive/", "/expectedDir-in-archive/with/", "/expectedDir-in-archive/with/nested/"} {
			header, err := tr.Next()
			h.AssertNil(t, err)

			h.AssertEq(t, header.Name, expectedDir)

			h.AssertEq(t, header.Uid, 1234)
			h.AssertEq(t, header.Gid, 2345)

			if header.Typeflag != tar.TypeDir {
				t.Fatalf(`expected %s to be a directory`, expectedDir)
			}
			if header.Mode != 0755 {
				t.Fatalf(`expected %s to have mode 0755`, expectedDir)
			}

			t.Log("handles regular files")
		}
	})
}
