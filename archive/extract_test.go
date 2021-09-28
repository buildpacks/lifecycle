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

	"github.com/buildpacks/lifecycle/archive"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestArchiveExtract(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "extract", testExtract, spec.Report(report.Terminal{}))
}

func testExtract(t *testing.T, when spec.G, it spec.S) {
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
		var pathModes = []archive.PathMode{
			{"root", os.ModeDir + 0755},
			{"root/readonly", os.ModeDir + 0500},
			{"root/readonly/readonlysub", os.ModeDir + 0500},
			{"root/readonly/readonlysub/somefile", 0444},
			{"root/standarddir", os.ModeDir + 0755},
			{"root/standarddir/somefile", 0644},
			{"root/symlinkdir", os.ModeSymlink + 0777},  //symlink permissions are not preserved from archive
			{"root/symlinkfile", os.ModeSymlink + 0777}, //symlink permissions are not preserved from archive
		}

		// Golang for Windows only implements owner permissions
		if runtime.GOOS == "windows" {
			pathModes = []archive.PathMode{
				{`root`, os.ModeDir + 0777},
				{`root\readonly`, os.ModeDir + 0555},
				{`root\readonly\readonlysub`, os.ModeDir + 0555},
				{`root\readonly\readonlysub\somefile`, 0444},
				{`root\standarddir`, os.ModeDir + 0777},
				{`root\standarddir\somefile`, 0666},
				{`root\symlinkdir`, os.ModeSymlink + 0666},
				{`root\symlinkfile`, os.ModeSymlink + 0666},
			}
		}

		it.Before(func() {
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
		})

		it.After(func() {
			// Make all files os.ModePerm so they can all be cleaned up.
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
		})

		it("extracts a tar file", func() {
			old := archive.SetUmask(0)
			defer archive.SetUmask(old)

			h.AssertNil(t, archive.Extract(tr))

			for _, pathMode := range pathModes {
				extractedFile := filepath.Join(tmpDir, pathMode.Path)

				fileInfo, err := os.Lstat(extractedFile)
				h.AssertNil(t, err)

				h.AssertEq(t, fileInfo.Mode(), pathMode.Mode)
			}
		})

		it("fails if file exists where directory needs to be created", func() {
			old := archive.SetUmask(0)
			defer archive.SetUmask(old)

			file, err := os.Create(filepath.Join(tmpDir, "root"))
			h.AssertNil(t, err)
			h.AssertNil(t, file.Close())

			h.AssertError(t, archive.Extract(tr), "failed to create directory")
		})

		it("doesn't alter permissions of existing folders", func() {
			old := archive.SetUmask(0)
			defer archive.SetUmask(old)

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

		when("umask is set", func() {
			it("errors", func() {
				currentMask, err := archive.GetUmask()
				h.AssertNil(t, err)
				h.SkipIf(t, currentMask == 0, "#Extract will not error as expected if umask is unset")

				err = archive.Extract(tr)
				h.AssertNotNil(t, err)
				h.AssertError(t, err, "umask should be unset by the calling function")
			})
		})
	})

	when("#GetUmask", func() {
		it("returns the umask for the process", func() {
			h.SkipIf(t, runtime.GOOS == "windows", "#GetUmask not implemented for Windows")

			foundUmask, err := archive.GetUmask()
			h.AssertNil(t, err)

			h.AssertEq(t, foundUmask, h.ExpectedUmask())
		})
	})
}
