package archive_test

import (
	"archive/tar"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/archive"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestTarWindows(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "tarWindows", testTarWindows, spec.Report(report.Terminal{}))
}

func testTarWindows(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir string
		tr     *archive.NormalizingTarReader
		ftr    *fakeTarReader
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "archive-extract-test")
		h.AssertNil(t, err)
		ftr = &fakeTarReader{}
		tr = archive.NewNormalizingTarReader(ftr)
		tr.PrependDir(tmpDir)
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(tmpDir))
	})

	when("#Extract", func() {
		it.Before(func() {
			ftr.pushHeader(&tar.Header{
				Name:     "root/symlinkdir",
				Typeflag: tar.TypeSymlink,
				Linkname: filepath.Join("..", "not-in-archive-dir"),
				Mode:     int64(os.ModeSymlink | 0755),
				PAXRecords: map[string]string{
					"MSWINDOWS.fileattr": strconv.FormatUint(uint64(syscall.FILE_ATTRIBUTE_DIRECTORY), 10),
				},
			})
			ftr.pushHeader(&tar.Header{
				Name:     "root",
				Typeflag: tar.TypeDir,
				Mode:     int64(os.ModeDir | 0755),
			})
		})

		it("sets dir attribute on windows directory symlinks", func() {
			h.AssertNil(t, archive.Extract(tr))

			extractedFile := filepath.Join(tmpDir, "root", "symlinkdir")
			t.Log("asserting on", extractedFile)
			fileInfo, err := os.Lstat(extractedFile)
			h.AssertNil(t, err)
			if fileInfo.Sys().(*syscall.Win32FileAttributeData).FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY == 0 {
				t.Fatalf("expected directory file attribute to be set on %s", extractedFile)
			}
		})
	})

	when("#Compress", func() {
		var (
			tw   archive.TarWriter
			file *os.File
		)
		it.Before(func() {
			var err error
			file, err = os.Create(filepath.Join(tmpDir, "tar_test-go.tar"))
			h.AssertNil(t, err)
			tw = &archive.NormalizingTarWriter{TarWriter: tar.NewWriter(file)}
		})

		it.After(func() {
			file.Close()
		})

		it("Add PAX headers to symlink directories", func() {
			h.AssertNil(t, archive.AddDirToArchive(tw, "testdata/dir-to-tar"))
			h.AssertNil(t, file.Close())

			file, err := os.Open(file.Name())
			h.AssertNil(t, err)
			defer file.Close()
			tr := tar.NewReader(file)

			_, err = tr.Next() // skip testdata/dir-to-tar
			h.AssertNil(t, err)

			tarContains(t, "directory symlinks", func() {
				header, err := tr.Next()
				h.AssertNil(t, err)

				h.AssertEq(t, header.Name, "testdata/dir-to-tar/dir-link")
				attrStr, ok := header.PAXRecords["MSWINDOWS.fileattr"]
				if !ok {
					t.Fatalf("Missing expected fileattr PAX record")
				}
				attr, err := strconv.ParseUint(attrStr, 10, 32)
				h.AssertNil(t, err)
				if attr&syscall.FILE_ATTRIBUTE_DIRECTORY == 0 {
					t.Fatalf("PAX records missing directory file attribute")
				}
			})
		})
	})
}
