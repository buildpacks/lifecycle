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
		file   *os.File
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "archive-extract-test")
		h.AssertNil(t, err)

		archivePath := filepath.Join("testdata", "tar-to-dir", "some-linux-layer.tar")
		if runtime.GOOS == "windows" {
			archivePath = filepath.Join("testdata", "tar-to-dir", "some-windows-layer.tar")
		}
		file, err = os.Open(archivePath)
		h.AssertNil(t, err)
		tr = archive.NewNormalizingTarReader(tar.NewReader(file))
		tr.PrependDir(tmpDir)
		tr.FromSlash()
	})

	it.After(func() {
		h.AssertNil(t, file.Close())
		h.AssertNil(t, os.RemoveAll(tmpDir))
	})

	when("#Extract", func() {
		var pathModes = []archive.PathMode{
			{"root", os.ModeDir + 0755},
			{"root/readonly", os.ModeDir + 0500},
			{"root/standarddir", os.ModeDir + 0755},
			{"root/standarddir/somefile", 0644},
			{"root/readonly/readonlysub/somefile", 0444},
			{"root/readonly/readonlysub", os.ModeDir + 0500},
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
				{`root\symlinkdir`, os.ModeDir + 0777},
				{`root\symlinkdir\subdir`, os.ModeDir + 0777},
				{`root\symlinkdir\subdir\somefile`, 0666},
				{`root\symlinkdir\symlink`, 0666},
				// other-dir/other-file intentionally left out, as it should not be extracted
			}
		}

		it.After(func() {
			// Make all files os.ModePerm so they can all be cleaned up.
			for _, pathMode := range pathModes {
				extractedFile := filepath.Join(tmpDir, pathMode.Path)
				if _, err := os.Stat(extractedFile); err == nil {
					if err := os.Chmod(extractedFile, os.ModePerm); err != nil {
						h.AssertNil(t, err)
					}
				}
			}
		})

		it("extracts a tar file", func() {
			h.AssertNil(t, archive.Extract(tr))

			for _, pathMode := range pathModes {
				extractedFile := filepath.Join(tmpDir, pathMode.Path)
				fileInfo, err := os.Stat(extractedFile)

				h.AssertNil(t, err)
				h.AssertEq(t, fileInfo.Mode(), pathMode.Mode)
			}
		})

		it("fails if file exists where directory needs to be created", func() {
			_, err := os.Create(filepath.Join(tmpDir, "root"))
			h.AssertNil(t, err)

			h.AssertError(t, archive.Extract(tr), "root: not a directory")
		})

		it("doesn't alter permissions of existing folders", func() {
			h.AssertNil(t, os.Mkdir(filepath.Join(tmpDir, "root"), 0744))
			// Update permissions in case umask was applied.
			h.AssertNil(t, os.Chmod(filepath.Join(tmpDir, "root"), 0744))

			h.AssertNil(t, archive.Extract(tr))
			fileInfo, err := os.Stat(filepath.Join(tmpDir, "root"))
			h.AssertNil(t, err)
			h.AssertEq(t, fileInfo.Mode(), os.ModeDir+0744)
		})
	})
}
