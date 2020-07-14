package layers_test

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/layers"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDirLayers(t *testing.T) {
	spec.Run(t, "Factory", testDirs, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testDirs(t *testing.T, when spec.G, it spec.S) {
	var (
		factory    *layers.Factory
		dir        string
		logHandler = memory.New()
	)
	it.Before(func() {
		var err error
		artifactDir, err := ioutil.TempDir("", "layers.slices.layer")
		h.AssertNil(t, err)
		factory = &layers.Factory{
			ArtifactsDir: artifactDir,
			Logger:       &log.Logger{Handler: logHandler},
			UID:          1234,
			GID:          4321,
		}
		dir, err = filepath.Abs(filepath.Join("testdata", "slices", "target-dir"))
		h.AssertNil(t, err)
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(factory.ArtifactsDir))
	})

	when("#DirLayer", func() {
		var dirLayer layers.Layer
		it.Before(func() {
			var err error
			dirLayer, err = factory.DirLayer("some-layer-id", dir)
			h.AssertNil(t, err)
		})

		it("creates a layer from the directory", func() {
			// parent layers should have uid/gid matching the filesystem
			// the dir and it's children should have normalized uid/gid
			assertTarEntries(t, dirLayer.TarPath, append(parents(t, dir), []entry{
				{
					name:     dir,
					uid:      factory.UID,
					gid:      factory.GID,
					typeFlag: tar.TypeDir,
				},
				{
					name:     filepath.Join(dir, "dir-link"),
					uid:      factory.UID,
					gid:      factory.GID,
					typeFlag: tar.TypeSymlink,
				},
				{
					name:     filepath.Join(dir, "file-link.txt"),
					uid:      factory.UID,
					gid:      factory.GID,
					typeFlag: tar.TypeSymlink,
				},
				{
					name:     filepath.Join(dir, "file.txt"),
					uid:      factory.UID,
					gid:      factory.GID,
					typeFlag: tar.TypeReg,
				},
				{
					name:     filepath.Join(dir, "other-dir"),
					uid:      factory.UID,
					gid:      factory.GID,
					typeFlag: tar.TypeDir,
				},
				{
					name:     filepath.Join(dir, "other-dir", "other-file.md"),
					uid:      factory.UID,
					gid:      factory.GID,
					typeFlag: tar.TypeReg,
				},
				{
					name:     filepath.Join(dir, "other-dir", "other-file.txt"),
					uid:      factory.UID,
					gid:      factory.GID,
					typeFlag: tar.TypeReg,
				},
				{
					name:     filepath.Join(dir, "some-dir"),
					uid:      factory.UID,
					gid:      factory.GID,
					typeFlag: tar.TypeDir,
				},
				{
					name:     filepath.Join(dir, "some-dir", "file.md"),
					uid:      factory.UID,
					gid:      factory.GID,
					typeFlag: tar.TypeReg,
				},
				{
					name:     filepath.Join(dir, "some-dir", "some-file.txt"),
					uid:      factory.UID,
					gid:      factory.GID,
					typeFlag: tar.TypeReg,
				},
			}...))
		})

		it("reuses tars when possible", func() {
			layerTar, err := os.Stat(dirLayer.TarPath)
			h.AssertNil(t, err)
			modTime := layerTar.ModTime()
			reusedDirLayer, err := factory.DirLayer("some-layer-id", dir)
			h.AssertNil(t, err)
			h.AssertEq(t, reusedDirLayer, dirLayer)
			layerTar, err = os.Stat(reusedDirLayer.TarPath)
			h.AssertNil(t, err)
			h.AssertEq(t, modTime, layerTar.ModTime()) // assert file has not been modified

			h.AssertEq(t, len(logHandler.Entries), 1)
			h.AssertEq(t,
				logHandler.Entries[0].Message,
				fmt.Sprintf("Reusing tarball for layer \"some-layer-id\" with SHA: %s\n", dirLayer.Digest),
			)
		})
	})
}

type entry struct {
	name     string
	uid, gid int
	typeFlag byte
}

func assertTarEntries(t *testing.T, tarPath string, expectedEntries []entry) {
	t.Helper()
	lf, err := os.Open(tarPath)
	h.AssertNil(t, err)
	defer lf.Close()
	tr := tar.NewReader(lf)

	for i, expected := range expectedEntries {
		header, err := tr.Next()
		if err == io.EOF {
			t.Fatalf("missing expected archive entry '%s'", expected.name)
		}
		h.AssertNil(t, err)
		if header.Name != expected.name {
			t.Fatalf("expected entry '%d' to have name %q, got %q", i, expected.name, header.Name)
		}
		if header.Typeflag != expected.typeFlag {
			t.Fatalf("expected entry '%s' to have type %q, got %q", header.Name, expected.typeFlag, header.Typeflag)
		}
		if header.Uid != expected.uid {
			t.Fatalf("expected entry '%s' to have UID %d, got %d", header.Name, expected.uid, header.Uid)
		}
		if header.Gid != expected.gid {
			t.Fatalf("expected entry '%s' to have GID %d, got %d", header.Name, expected.gid, header.Gid)
		}
		if !header.ModTime.Equal(time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)) {
			t.Fatalf("expected entry '%s' to normalized mod time, got '%s", header.Name, header.ModTime)
		}
		if header.Uname != "" {
			t.Fatalf("expected entry '%s' to empty Uname, got '%s", header.Name, header.Uname)
		}
		if header.Gname != "" {
			t.Fatalf("expected entry '%s' to empty Gname, got '%s", header.Name, header.Gname)
		}
		h.AssertEq(t, header.Typeflag, expected.typeFlag)
	}
	header, err := tr.Next()
	if err != io.EOF {
		t.Fatalf("unexpected archive entry '%s'", header.Name)
	}
}

func parents(t *testing.T, file string) []entry {
	t.Helper()
	parent := filepath.Dir(file)
	stat, err := os.Stat(parent)
	sys := stat.Sys().(*syscall.Stat_t)
	h.AssertNil(t, err)
	fileEntry := entry{
		name:     parent,
		uid:      int(sys.Uid),
		gid:      int(sys.Gid),
		typeFlag: tar.TypeDir,
	}
	if parent == "/" {
		return []entry{}
	}
	return append(parents(t, parent), fileEntry)
}
