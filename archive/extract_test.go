package archive_test

import (
	"archive/tar"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
	"golang.org/x/sync/errgroup"

	"github.com/buildpacks/lifecycle/archive"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var originalUmask int

func TestArchiveExtract(t *testing.T) {
	originalUmask = h.GetUmask(t)
	spec.Run(t, "extract", testExtract, spec.Report(report.Terminal{}))
}

func testExtract(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir    string
		destRoot  string
		tr        *archive.NormalizingTarReader
		pathModes []archive.PathMode
	)

	it.Before(func() {
		tr, tmpDir = newFakeTarReader(t)
		destRoot = tmpDir
		pathModes = []archive.PathMode{
			{"root", os.ModeDir + 0755},
			{"root/readonly", os.ModeDir + 0500},
			{"root/readonly/readonlysub", os.ModeDir + 0500},
			{"root/readonly/readonlysub/somefile", 0444},
			{"root/standarddir", os.ModeDir + 0755},
			{"root/standarddir/somefile", 0644},
			{"root/nonexistdirnotintar", os.ModeDir + os.FileMode(int(os.ModePerm)&^originalUmask)}, //nolint
			{"root/symlinkdir", os.ModeSymlink + 0777},                                              // symlink permissions are not preserved from archive
			{"root/symlinkfile", os.ModeSymlink + 0777},                                             // symlink permissions are not preserved from archive
		}
	})

	it.After(func() {
		// Make all files os.ModePerm so they can all be cleaned up.
		cleanupTmpDir(t, tmpDir, pathModes)
	})

	when("#Extract", func() {
		it("extracts a tar file", func() {
			h.AssertNil(t, archive.Extract(tr, destRoot))

			for _, pathMode := range pathModes {
				testPathPerms(t, tmpDir, pathMode.Path, pathMode.Mode)
			}
		})

		it("thread safe", func() {
			tr2, tmpDir2 := newFakeTarReader(t)
			tr3, tmpDir3 := newFakeTarReader(t)

			var g errgroup.Group
			g.Go(func() error { return archive.Extract(tr, destRoot) })
			g.Go(func() error { return archive.Extract(tr2, tmpDir2) })
			g.Go(func() error { return archive.Extract(tr3, tmpDir3) })

			h.AssertNil(t, g.Wait())
			h.AssertEq(t, h.GetUmask(t), originalUmask)

			cleanupTmpDir(t, tmpDir2, pathModes)
			cleanupTmpDir(t, tmpDir3, pathModes)
		})

		it("fails if file exists where directory needs to be created", func() {
			file, err := os.Create(filepath.Join(tmpDir, "root"))
			h.AssertNil(t, err)
			h.AssertNil(t, file.Close())

			h.AssertError(t, archive.Extract(tr, destRoot), "failed to create directory")
		})

		it("doesn't alter permissions of existing folders", func() {
			h.AssertNil(t, os.Mkdir(filepath.Join(tmpDir, "root"), 0744))
			// Update permissions in case umask was applied.
			h.AssertNil(t, os.Chmod(filepath.Join(tmpDir, "root"), 0744))

			h.AssertNil(t, archive.Extract(tr, destRoot))
			fileInfo, err := os.Stat(filepath.Join(tmpDir, "root"))
			h.AssertNil(t, err)

			h.AssertEq(t, fileInfo.Mode(), os.ModeDir+0744)
		})

		it("does not write tar entries outside of the destination directory", func() {
			parentDir, err := os.MkdirTemp("", "escape-parent")
			h.AssertNil(t, err)
			defer func() {
				_ = os.RemoveAll(parentDir)
			}()

			destDir := filepath.Join(parentDir, "dest")
			h.AssertNil(t, os.MkdirAll(destDir, 0750))

			ftr := &fakeTarReader{}
			ftr.pushHeader(&tar.Header{
				Name:     "../escaped-file",
				Typeflag: tar.TypeReg,
				Mode:     int64(0644),
			})
			evilTr := archive.NewNormalizingTarReader(ftr)
			evilTr.PrependDir(destDir)

			h.AssertError(t, archive.Extract(evilTr, destDir), "escapes destination root")

			escapedPath := filepath.Join(parentDir, "escaped-file")
			_, err = os.Lstat(escapedPath)
			h.AssertError(t, err, "no such file or directory")
		})

		when("a tar entry writes through a symlink that leaves the destination", func() {
			it("returns an error", func() {
				parentDir, err := os.MkdirTemp("", "symlink-escape-parent")
				h.AssertNil(t, err)
				defer func() { _ = os.RemoveAll(parentDir) }()

				destDir := filepath.Join(parentDir, "dest")
				h.AssertNil(t, os.MkdirAll(destDir, 0750))

				ftr2 := &fakeTarReader{}
				tr2 := archive.NewNormalizingTarReader(ftr2)
				tr2.PrependDir(destDir)
				ftr2.pushHeader(&tar.Header{Name: "foo/bar/baz", Typeflag: tar.TypeReg, Mode: int64(0644)})
				ftr2.pushHeader(&tar.Header{Name: "foo/bar", Typeflag: tar.TypeSymlink, Linkname: "../../escaped"})
				ftr2.pushHeader(&tar.Header{Name: "foo", Typeflag: tar.TypeDir, Mode: int64(os.ModeDir | 0755)})

				h.AssertError(t, archive.Extract(tr2, destDir), "failed to write file")
				_, err = os.Lstat(filepath.Join(parentDir, "escaped"))
				h.AssertError(t, err, "no such file or directory")
			})
		})

		when("a symlink entry points outside the destination", func() {
			it("creates it faithfully", func() {
				ftr2 := &fakeTarReader{}
				tr2 := archive.NewNormalizingTarReader(ftr2)
				tr2.PrependDir(tmpDir)
				ftr2.pushHeader(&tar.Header{Name: "outward", Typeflag: tar.TypeSymlink, Linkname: "../../elsewhere"})

				h.AssertNil(t, archive.Extract(tr2, tmpDir))

				target, err := os.Readlink(filepath.Join(tmpDir, "outward"))
				h.AssertNil(t, err)
				h.AssertEq(t, target, "../../elsewhere")
			})
		})

		when("a regular-file entry targets a pre-existing symlink", func() {
			it("does not follow it", func() {
				outside, err := os.MkdirTemp("", "nofollow-outside")
				h.AssertNil(t, err)
				defer func() { _ = os.RemoveAll(outside) }()
				victim := filepath.Join(outside, "victim")

				// Pre-plant a symlink inside destRoot pointing outside.
				h.AssertNil(t, os.Symlink(victim, filepath.Join(tmpDir, "planted")))

				ftr := &fakeTarReader{}
				tr2 := archive.NewNormalizingTarReader(ftr)
				tr2.PrependDir(tmpDir)
				ftr.pushHeader(&tar.Header{Name: "planted", Typeflag: tar.TypeReg, Mode: int64(0644)})

				h.AssertError(t, archive.Extract(tr2, tmpDir), "planted")
				_, err = os.Lstat(victim)
				h.AssertError(t, err, "no such file or directory")
			})
		})
	})
}

func newFakeTarReader(t *testing.T) (*archive.NormalizingTarReader, string) {
	tmpDir, err := os.MkdirTemp("", "archive-extract-test")
	h.AssertNil(t, err)
	ftr := &fakeTarReader{}
	tr := archive.NewNormalizingTarReader(ftr)
	tr.PrependDir(tmpDir)
	pushHeaders(ftr)
	return tr, tmpDir
}

func pushHeaders(ftr *fakeTarReader) {
	ftr.pushHeader(&tar.Header{
		Name:     "pax_global_header",
		Typeflag: tar.TypeXGlobalHeader,
		Mode:     int64(0),
	})
	ftr.pushHeader(&tar.Header{
		Name:     "root/symlinkdir",
		Typeflag: tar.TypeSymlink,
		Linkname: filepath.Join("..", "not-in-archive-dir"),
		Mode:     int64(os.ModeSymlink | 0755),
	})
	ftr.pushHeader(&tar.Header{
		Name:     "root/symlinkfile",
		Typeflag: tar.TypeSymlink,
		Linkname: filepath.FromSlash("../not-in-archive-file"),
		Mode:     int64(os.ModeSymlink | 0755),
	})
	ftr.pushHeader(&tar.Header{
		Name:     "root/nonexistdirnotintar/somefile",
		Typeflag: tar.TypeReg,
		Mode:     int64(0644),
	})
	ftr.pushHeader(&tar.Header{
		Name:     "root/standarddir/somefile",
		Typeflag: tar.TypeReg,
		Mode:     int64(0644),
	})
	ftr.pushHeader(&tar.Header{
		Name:     "root/standarddir",
		Typeflag: tar.TypeDir,
		Mode:     int64(os.ModeDir | 0755),
	})
	ftr.pushHeader(&tar.Header{
		Name:     "root/readonly/readonlysub/somefile",
		Typeflag: tar.TypeReg,
		Mode:     int64(0444),
	})
	ftr.pushHeader(&tar.Header{
		Name:     "root/readonly/readonlysub",
		Typeflag: tar.TypeDir,
		Mode:     int64(os.ModeDir | 0500),
	})
	ftr.pushHeader(&tar.Header{
		Name:     "root/readonly",
		Typeflag: tar.TypeDir,
		Mode:     int64(os.ModeDir | 0500),
	})
	ftr.pushHeader(&tar.Header{
		Name:     "root",
		Typeflag: tar.TypeDir,
		Mode:     int64(os.ModeDir | 0755),
	})
}

func cleanupTmpDir(t *testing.T, tmpDir string, pathModes []archive.PathMode) {
	for _, pathMode := range pathModes {
		extractedFile := filepath.Join(tmpDir, pathMode.Path)
		if fi, err := os.Lstat(extractedFile); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if err := os.Chmod(extractedFile, os.ModePerm); err != nil {
				h.AssertNil(t, err)
			}
		}
	}
	h.AssertNil(t, os.RemoveAll(tmpDir))
}

func testPathPerms(t *testing.T, parentDir, path string, expectedMode os.FileMode) {
	extractedFile := filepath.Join(parentDir, path)

	fileInfo, err := os.Lstat(extractedFile)
	h.AssertNil(t, err)

	if fileInfo.Mode() != expectedMode {
		t.Fatalf("unexpected permissions for %s; got: %o, want: %o", path, fileInfo.Mode(), expectedMode)
	}
}
