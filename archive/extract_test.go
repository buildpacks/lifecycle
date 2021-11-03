package archive_test

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
	"golang.org/x/sync/errgroup"

	"github.com/buildpacks/lifecycle/archive"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

var originalUmask int

func TestArchiveExtract(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	originalUmask = h.GetUmask(t)
	spec.Run(t, "extract", testExtract, spec.Report(report.Terminal{}))
}

func testExtract(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir    string
		tr        *archive.NormalizingTarReader
		pathModes []archive.PathMode
	)

	it.Before(func() {
		tr, tmpDir = newFakeTarReader(t)
		// Golang for Windows only implements owner permissions
		if runtime.GOOS == "windows" {
			pathModes = []archive.PathMode{
				{`root`, os.ModeDir + 0777},
				{`root\readonly`, os.ModeDir + 0555},
				{`root\readonly\readonlysub`, os.ModeDir + 0555},
				{`root\readonly\readonlysub\somefile`, 0444},
				{`root\standarddir`, os.ModeDir + 0777},
				{`root\standarddir\somefile`, 0666},
				{`root\nonexistdirnotintar`, os.ModeDir + 0777},
				{`root\symlinkdir`, os.ModeSymlink + 0666},
				{`root\symlinkfile`, os.ModeSymlink + 0666},
			}
		} else {
			pathModes = []archive.PathMode{
				{"root", os.ModeDir + 0755},
				{"root/readonly", os.ModeDir + 0500},
				{"root/readonly/readonlysub", os.ModeDir + 0500},
				{"root/readonly/readonlysub/somefile", 0444},
				{"root/standarddir", os.ModeDir + 0755},
				{"root/standarddir/somefile", 0644},
				{"root/nonexistdirnotintar", os.ModeDir + os.FileMode(int(os.ModePerm)&^originalUmask)},
				{"root/symlinkdir", os.ModeSymlink + 0777},  // symlink permissions are not preserved from archive
				{"root/symlinkfile", os.ModeSymlink + 0777}, // symlink permissions are not preserved from archive
			}
		}
	})

	it.After(func() {
		// Make all files os.ModePerm so they can all be cleaned up.
		cleanupTmpDir(t, tmpDir, pathModes)
	})

	when("#Extract", func() {
		it("extracts a tar file", func() {
			h.AssertNil(t, archive.Extract(tr))

			for _, pathMode := range pathModes {
				testPathPerms(t, tmpDir, pathMode.Path, pathMode.Mode)
			}
		})

		it("thread safe", func() {
			tr2, tmpDir2 := newFakeTarReader(t)
			tr3, tmpDir3 := newFakeTarReader(t)

			var g errgroup.Group
			tars := []*archive.NormalizingTarReader{tr, tr2, tr3}
			for _, tarReader := range tars {
				tarReader := tarReader
				g.Go(func() error {
					return archive.Extract(tarReader)
				})
			}

			h.AssertNil(t, g.Wait())
			h.AssertEq(t, h.GetUmask(t), originalUmask)

			cleanupTmpDir(t, tmpDir2, pathModes)
			cleanupTmpDir(t, tmpDir3, pathModes)
		})

		it("fails if file exists where directory needs to be created", func() {
			file, err := os.Create(filepath.Join(tmpDir, "root"))
			h.AssertNil(t, err)
			h.AssertNil(t, file.Close())

			h.AssertError(t, archive.Extract(tr), "failed to create directory")
		})

		it("doesn't alter permissions of existing folders", func() {
			h.AssertNil(t, os.Mkdir(filepath.Join(tmpDir, "root"), 0744))
			// Update permissions in case umask was applied.
			h.AssertNil(t, os.Chmod(filepath.Join(tmpDir, "root"), 0744))

			h.AssertNil(t, archive.Extract(tr))
			fileInfo, err := os.Stat(filepath.Join(tmpDir, "root"))
			h.AssertNil(t, err)

			if runtime.GOOS != "windows" {
				h.AssertEq(t, fileInfo.Mode(), os.ModeDir+0744)
			} else {
				// Golang for Windows only implements owner permissions
				h.AssertEq(t, fileInfo.Mode(), os.ModeDir+0777)
			}
		})
	})
}

func newFakeTarReader(t *testing.T) (*archive.NormalizingTarReader, string) {
	tmpDir, err := ioutil.TempDir("", "archive-extract-test")
	h.AssertNil(t, err)
	ftr := &fakeTarReader{}
	tr := archive.NewNormalizingTarReader(ftr)
	tr.PrependDir(tmpDir)
	pushHeaders(ftr)
	return tr, tmpDir
}

func pushHeaders(ftr *fakeTarReader) {
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
