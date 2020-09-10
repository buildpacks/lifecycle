// +build linux darwin

package snapshot_test

import (
	"archive/tar"
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/snapshot"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestKanikoSnapshotter(t *testing.T) {
	spec.Run(t, "Test Image", testKanikoSnapshotter)
}

func testKanikoSnapshotter(t *testing.T, when spec.G, it spec.S) {
	var (
		snapshotter *snapshot.KanikoSnapshotter
		tmpDir      string
	)

	it.Before(func() {
		// Using the default tmp dir causes kaniko to go haywire for some reason
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		tmpDir, err = ioutil.TempDir(filepath.Join(cwd, "..", "tmp"), "kaniko")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}

		createTestFile(t, filepath.Join(tmpDir, "cnb", "privatefile"))
		createTestFile(t, filepath.Join(tmpDir, "layers", "privatefile"))
		createTestFile(t, filepath.Join(tmpDir, "file-to-change"))
		createTestFile(t, filepath.Join(tmpDir, "file-not-to-change"))
		createTestFile(t, filepath.Join(tmpDir, "file-to-delete"))
		createTestFile(t, filepath.Join(tmpDir, "bin", "file-not-to-change"))

		snapshotter = &snapshot.KanikoSnapshotter{
			RootDir:                    tmpDir,
			DetectFilesystemIgnoreList: false,
		}
		snapshotter.IgnoredPaths = []string{filepath.Join(snapshotter.RootDir, "dir-with-ignored-files")}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("files are added and modified", func() {
		var (
			snapshotFile string
		)

		it.Before(func() {
			h.AssertNil(t, snapshotter.Init())

			os.Remove(filepath.Join(snapshotter.RootDir, "file-to-delete"))
			createTestFileWithContent(t, filepath.Join(snapshotter.RootDir, "file-to-change"), "hola\n")
			createTestFile(t, filepath.Join(snapshotter.RootDir, "my-space", "newfile-in-dir"))
			createTestFile(t, filepath.Join(snapshotter.RootDir, "dir-with-ignored-files", "file-to-ignore"))

			tmpFile, err := ioutil.TempFile("", "snapshot")
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}

			snapshotFile = tmpFile.Name()

			err = snapshotter.TakeSnapshot(snapshotFile)
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
		})

		it("includes the changed files in the snapshot", func() {
			data, err := os.Open(snapshotFile)
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			defer data.Close()

			tr := tar.NewReader(data)
			for {
				hdr, err := tr.Next()
				if err == io.EOF {
					break // End of archive
				}

				if err != nil {
					t.Fatalf("Error: %s\n", err)
				}

				switch hdr.Name {
				case "/":
				case "dir-with-ignored-files/":
				case "my-space/":
				case strings.Trim(filepath.Join(snapshotter.RootDir, ".wh.file-to-delete"), "/"):
					continue
				case "newfile":
				case "my-space/newfile-in-dir":
					assertSnapshotFile(t, tr, "hello\n")
				case "file-to-change":
					assertSnapshotFile(t, tr, "hola\n")
				default:
					t.Fatalf("Unexpected file: %s\n", hdr.Name)
				}
			}
		})
	})
}

func mkdir(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0777); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func createTestFile(t *testing.T, path string) {
	createTestFileWithContent(t, path, "hello\n")
}

func createTestFileWithContent(t *testing.T, path string, content string) {
	mkdir(t, filepath.Dir(path))

	data := []byte(content)

	if err := ioutil.WriteFile(path, data, 0777); err != nil {
		t.Fatalf("Error: %s", err)
	}
}

func assertSnapshotFile(t *testing.T, tr *tar.Reader, content string) {
	var b bytes.Buffer
	if _, err := io.Copy(&b, tr); err != nil {
		t.Fatalf("Unexpected info:\n%s\n", err)
	}

	if s := cmp.Diff(b.String(), content); s != "" {
		t.Fatalf("Unexpected info:\n%s\n", s)
	}
}
