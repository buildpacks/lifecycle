package archive_test

import (
	"archive/tar"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/archive"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestTar(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "tar", testTar, spec.Report(report.Terminal{}))
}

func testTar(t *testing.T, when spec.G, it spec.S) {
	var tmpDir string

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "extract-tar-test")
		h.AssertNil(t, err)
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(tmpDir))
	})

	when("#UntarLayer", func() {
		layerTar := "some-linux-layer.tar"
		pathModes := []archive.PathMode{
			{"root", os.ModeDir + 0755},
			{"root/readonly", os.ModeDir + 0500},
			{"root/readonly/readonlysub", os.ModeDir + 0500},
			{"root/readonly/readonlysub/somefile", 0444},
			{"root/standarddir", os.ModeDir + 0755},
			{"root/standarddir/somefile", 0644},
			{"root/symlinkdir", os.ModeDir + 0755},
			{"root/symlinkdir/subdir", os.ModeDir + 0755},
			{"root/symlinkdir/subdir/somefile", 0644},
			{"root/symlinkdir/symlink", 0644},
		}

		// Golang for Windows only implements owner permissions
		if runtime.GOOS == "windows" {
			layerTar = "some-windows-layer.tar"
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
				{`standardfile_Files`, 0666},
				// `standardfile_Hives` intentionally left out
			}
		}

		it.After(func() {
			// Make all files os.ModePerm so they can all be cleaned up.
			for _, pathMode := range pathModes {
				extractedFile := filepath.Join(tmpDir, pathMode.Path)
				if _, err := os.Stat(extractedFile); err == nil {
					h.AssertNil(t, os.Chmod(extractedFile, os.ModePerm))
				}
			}

			err := os.RemoveAll(tmpDir)
			h.AssertNil(t, err)
		})

		it("extracts a tar file", func() {
			file, err := os.Open(filepath.Join("testdata", "tar-to-dir", layerTar))
			h.AssertNil(t, err)
			defer file.Close()

			h.AssertNil(t, archive.UntarLayer(file, tmpDir))

			files := tree(t, tmpDir)
			if len(files) != len(pathModes) {
				var expected []string
				for _, p := range pathModes {
					expected = append(expected, p.Path)
				}
				t.Fatalf("Extracted wrong number of files:\nExpected:\n- %s\n\nGot:\n- %s", strings.Join(expected, "\n- "), strings.Join(files, "\n- "))
			}

			for _, pathMode := range pathModes {
				extractedFile := filepath.Join(tmpDir, pathMode.Path)
				fileInfo, err := os.Stat(extractedFile)
				h.AssertNil(t, err)

				if fileInfo.Mode() != pathMode.Mode {
					t.Fatalf("Unexpected mode for '%s': expected %o but got %o\n", extractedFile, pathMode.Mode, fileInfo.Mode())
				}
				h.AssertEq(t, fileInfo.Mode(), pathMode.Mode)
			}
		})

		it("fails if file exists where directory needs to be created", func() {
			file, err := os.Create(filepath.Join(tmpDir, "root"))
			h.AssertNil(t, err)
			defer file.Close()

			file, err = os.Open(filepath.Join("testdata", "tar-to-dir", layerTar))
			h.AssertNil(t, err)
			defer file.Close()

			if runtime.GOOS != "windows" {
				h.AssertError(t, archive.UntarLayer(file, tmpDir), "root: not a directory")
			} else {
				h.AssertError(t, archive.UntarLayer(file, tmpDir), "root: The system cannot find the path specified.")
			}
		})

		it("doesn't alter permissions of existing folders", func() {
			h.AssertNil(t, os.Mkdir(filepath.Join(tmpDir, "root"), 0744))
			// Update permissions in case umask was applied.
			h.AssertNil(t, os.Chmod(filepath.Join(tmpDir, "root"), 0744))

			file, err := os.Open(filepath.Join("testdata", "tar-to-dir", layerTar))
			h.AssertNil(t, err)
			defer file.Close()

			h.AssertNil(t, archive.UntarLayer(file, tmpDir))
			fileInfo, err := os.Stat(filepath.Join(tmpDir, "root"))
			h.AssertNil(t, err)

			if runtime.GOOS != "windows" {
				h.AssertEq(t, fileInfo.Mode(), os.ModeDir+0744)
			} else {
				// Golang for Windows only implements owner permissions
				h.AssertEq(t, fileInfo.Mode(), os.ModeDir+0777)
			}
		})

		it("preserves symlinks", func() {
			file, err := os.Open(filepath.Join("testdata", "tar-to-dir", layerTar))
			h.AssertNil(t, err)
			defer file.Close()

			h.AssertNil(t, archive.UntarLayer(file, tmpDir))

			contents, err := ioutil.ReadFile(filepath.Join(tmpDir, "root", "symlinkdir", "symlink"))
			h.AssertNil(t, err)
			h.AssertEq(t, string(contents), "some-content\n")

			link, err := os.Readlink(filepath.Join(tmpDir, "root", "symlinkdir", "symlink"))
			h.AssertNil(t, err)

			if runtime.GOOS != "windows" {
				h.AssertEq(t, link, "./subdir/somefile")
			} else {
				h.AssertEq(t, link, `.\subdir\somefile`)
			}
		})
	})

	when("#WriteTarArchive", func() {
		for _, src := range []string{
			filepath.Join("testdata", "dir-to-tar"),
			filepath.Join("testdata", "dir-to-tar") + string(filepath.Separator),
			filepath.Join("testdata", "dir-to-tar") + string(filepath.Separator) + ".",
		} {
			src := src
			it(fmt.Sprintf("writes a tar with the src filesystem contents (%s)", src), func() {
				uid := 1234
				gid := 4567

				file, err := os.Create(filepath.Join(tmpDir, "tar_test-go.tar"))
				h.AssertNil(t, err)
				defer file.Close()

				h.AssertNil(t, archive.WriteTarArchive(file, archive.DefaultTarWriterFactory(), src, uid, gid))
				h.AssertNil(t, file.Close())

				file, err = os.Open(file.Name())
				h.AssertNil(t, err)

				defer file.Close()
				tr := tar.NewReader(file)

				tarContains(t, func() {
					header, err := tr.Next()
					h.AssertNil(t, err)
					h.AssertEq(t, header.Name, "testdata")
					assertModTimeNormalized(t, header)

					header, err = tr.Next()
					h.AssertNil(t, err)
					h.AssertEq(t, header.Name, "testdata/dir-to-tar")
					assertModTimeNormalized(t, header)
				})

				tarContains(t, func() {
					header, err := tr.Next()
					h.AssertNil(t, err)
					h.AssertEq(t, header.Name, "testdata/dir-to-tar/some-file.txt")

					fileContents := make([]byte, header.Size)
					tr.Read(fileContents)
					h.AssertEq(t, string(fileContents), "some-content")
					h.AssertEq(t, header.Uid, uid)
					h.AssertEq(t, header.Gid, gid)
					assertModTimeNormalized(t, header)
				})

				tarContains(t, func() {
					header, err := tr.Next()
					h.AssertNil(t, err)
					h.AssertEq(t, header.Name, "testdata/dir-to-tar/sub-dir")
					assertModTimeNormalized(t, header)
				})

				tarContains(t, func() {
					header, err := tr.Next()
					h.AssertNil(t, err)

					h.AssertEq(t, header.Name, "testdata/dir-to-tar/sub-dir/link-file")
					h.AssertEq(t, header.Uid, uid)
					h.AssertEq(t, header.Gid, gid)
					h.AssertEq(t, header.Linkname, "../some-file.txt")
					assertModTimeNormalized(t, header)
				})
			})
		}

		when("an absolute path is given", func() {
			it("has working test helpers", func() {
				h.AssertEq(t,
					allParentDirectories(filepath.FromSlash("/some/absolute/directory")),
					[]string{filepath.FromSlash("/some"), filepath.FromSlash("/some/absolute")},
				)
			})

			it("writes headers for all parent directories if an absolute path is given", func() {
				absoluteFilePath, err := filepath.Abs(filepath.Join("testdata", "dir-to-tar"))
				h.AssertNil(t, err)

				file, err := os.Create(filepath.Join(tmpDir, "tar_test-go.tar"))
				h.AssertNil(t, err)
				defer file.Close()

				h.AssertNil(t, archive.WriteTarArchive(file, archive.DefaultTarWriterFactory(), absoluteFilePath, 1234, 5678))
				h.AssertNil(t, file.Close())

				file, err = os.Open(file.Name())
				h.AssertNil(t, err)

				defer file.Close()
				tr := tar.NewReader(file)

				for _, expectedDir := range allParentDirectories(absoluteFilePath) {
					header, err := tr.Next()
					h.AssertNil(t, err)

					h.AssertEq(t, header.Name, archive.TarPath(expectedDir))

					assertDirectory(t, header)
					assertModTimeNormalized(t, header)
				}
			})
		})

		when("a relative path is given", func() {
			it("has working test helpers", func() {
				h.AssertEq(t,
					allParentDirectories(filepath.Join("some", "relative", "path")),
					[]string{"some", filepath.Join("some", "relative")},
				)
			})

			it("writes headers for all parent directories", func() {
				relativePath := filepath.Join("testdata", "dir-to-tar", "sub-dir")

				file, err := os.Create(filepath.Join(tmpDir, "tar_test-go.tar"))
				h.AssertNil(t, err)
				defer file.Close()

				h.AssertNil(t, archive.WriteTarArchive(file, archive.DefaultTarWriterFactory(), relativePath, 1234, 5678))
				h.AssertNil(t, file.Close())

				file, err = os.Open(file.Name())
				h.AssertNil(t, err)
				defer file.Close()

				tr := tar.NewReader(file)
				for _, expectedDir := range allParentDirectories(relativePath) {
					header, err := tr.Next()
					h.AssertNil(t, err)

					h.AssertEq(t, header.Name, archive.TarPath(expectedDir))

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

			file, err := os.Create(filepath.Join(tmpDir, "tar_test-go.tar"))
			h.AssertNil(t, err)
			defer file.Close()

			h.AssertNil(t, archive.WriteTarArchive(file, archive.DefaultTarWriterFactory(), src, 1234, 5678))
			h.AssertNil(t, file.Close())

			file, err = os.Open(file.Name())
			h.AssertNil(t, err)

			defer file.Close()
			tr := tar.NewReader(file)

			for _, expectedDir := range allParentDirectories(src) {
				header, err := tr.Next()
				h.AssertNil(t, err)
				h.AssertEq(t, header.Name, archive.TarPath(expectedDir))

				assertDirectory(t, header)

				localDir, err := os.Stat(expectedDir)
				h.AssertNil(t, err)

				assertPermissions(t, header, localDir.Mode().Perm())
			}
		})
	})
}

func tarContains(t *testing.T, r func()) {
	t.Helper()
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
	if parent == "." || parent == filepath.VolumeName(directory)+string(filepath.Separator) {
		return []string{}
	}
	return append(allParentDirectories(parent), parent)
}

func tree(t *testing.T, directory string) []string {
	t.Helper()
	var files []string
	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != directory {
			files = append(files, strings.TrimPrefix(path, directory+string(filepath.Separator)))
		}
		return nil
	})
	h.AssertNil(t, err)
	return files
}
