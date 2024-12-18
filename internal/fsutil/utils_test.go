package fsutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/internal/fsutil"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestIO(t *testing.T) {
	spec.Run(t, "IO", testIO, spec.Report(report.Terminal{}))
}

func testIO(t *testing.T, when spec.G, it spec.S) {
	var tmpDir string

	it.Before(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "lifecycle.test")
		h.AssertNil(t, err)
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#Copy", func() {
		when("called with file", func() {
			it("copies source to destination", func() {
				src := filepath.Join(tmpDir, "src.txt")
				dst := filepath.Join(tmpDir, "dest.txt")
				h.Mkfile(t, "some-file-content", src)

				h.AssertNil(t, fsutil.Copy(src, dst))

				result := h.MustReadFile(t, dst)
				h.AssertEq(t, string(result), "some-file-content")
			})
		})

		when("called with directory", func() {
			it("copies source to destination", func() {
				src := filepath.Join("testdata", "some_dir")
				dst := filepath.Join(tmpDir, "dest_dir")

				h.AssertNil(t, fsutil.Copy(src, dst))

				h.AssertPathExists(t, dst)
				h.AssertPathExists(t, filepath.Join(dst, "some_file"))
				contents := h.MustReadFile(t, filepath.Join(dst, "some_file"))
				h.AssertEq(t, string(contents), "some-content\n")
				h.AssertPathExists(t, filepath.Join(dst, "some_link"))
				target, err := os.Readlink(filepath.Join(dst, "some_link"))
				h.AssertNil(t, err)
				h.AssertEq(t, target, "some_file")
				h.AssertPathExists(t, filepath.Join(dst, "other_dir"))
				h.AssertPathExists(t, filepath.Join(dst, "other_dir", "other_file"))
				contents = h.MustReadFile(t, filepath.Join(dst, "other_dir", "other_file"))
				h.AssertEq(t, string(contents), "other-content\n")
				h.AssertPathExists(t, filepath.Join(dst, "other_dir", "other_link"))
				target, err = os.Readlink(filepath.Join(dst, "other_dir", "other_link"))
				h.AssertEq(t, target, "other_file")
				h.AssertNil(t, err)
			})
		})
	})

	when("#FilesWithExtensions", func() {
		when("called with directory and extensions", func() {
			it("filters the files", func() {
				src := "testdata"
				exts := []string{".txt", ".json"}
				files, err := fsutil.FilesWithExtensions(src, exts)
				h.AssertNil(t, err)
				h.AssertEq(t, len(files), 2)
				h.AssertContains(t, files, filepath.Join(src, "file2.txt"), filepath.Join(src, "file3.json"))
			})
		})
	})
}
