package layers_test

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
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
		dir, err = filepath.Abs(filepath.Join("testdata", "target-dir"))
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
			h.AssertEq(t, dirLayer.ID, "some-layer-id")
			assertTarEntries(t, dirLayer.TarPath, append(parents(t, dir), []*tar.Header{
				{
					Name:     tarPath(dir),
					Uid:      factory.UID,
					Gid:      factory.GID,
					Typeflag: tar.TypeDir,
				},
				{
					Name:     tarPath(filepath.Join(dir, "dir-link")),
					Uid:      factory.UID,
					Gid:      factory.GID,
					Typeflag: tar.TypeSymlink,
				},
				{
					Name:     tarPath(filepath.Join(dir, "file-link.txt")),
					Uid:      factory.UID,
					Gid:      factory.GID,
					Typeflag: tar.TypeSymlink,
				},
				{
					Name:     tarPath(filepath.Join(dir, "file.txt")),
					Uid:      factory.UID,
					Gid:      factory.GID,
					Typeflag: tar.TypeReg,
				},
				{
					Name:     tarPath(filepath.Join(dir, "other-dir")),
					Uid:      factory.UID,
					Gid:      factory.GID,
					Typeflag: tar.TypeDir,
				},
				{
					Name:     tarPath(filepath.Join(dir, "other-dir", "other-file.md")),
					Uid:      factory.UID,
					Gid:      factory.GID,
					Typeflag: tar.TypeReg,
				},
				{
					Name:     tarPath(filepath.Join(dir, "other-dir", "other-file.txt")),
					Uid:      factory.UID,
					Gid:      factory.GID,
					Typeflag: tar.TypeReg,
				},
				{
					Name:     tarPath(filepath.Join(dir, "some-dir")),
					Uid:      factory.UID,
					Gid:      factory.GID,
					Typeflag: tar.TypeDir,
				},
				{
					Name:     tarPath(filepath.Join(dir, "some-dir", "file.md")),
					Uid:      factory.UID,
					Gid:      factory.GID,
					Typeflag: tar.TypeReg,
				},
				{
					Name:     tarPath(filepath.Join(dir, "some-dir", "some-file.txt")),
					Uid:      factory.UID,
					Gid:      factory.GID,
					Typeflag: tar.TypeReg,
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

func assertTarEntries(t *testing.T, tarPath string, expectedEntries []*tar.Header) {
	t.Helper()
	lf, err := os.Open(tarPath)
	h.AssertNil(t, err)
	defer lf.Close()
	tr := tar.NewReader(lf)
	assertOSSpecificEntries(t, tr)
	var allEntryNames []string
	for i, expected := range expectedEntries {
		header, err := tr.Next()
		if err == io.EOF {
			t.Fatalf("missing expected archive entry '%s'\n archive contained %v", expected.Name, allEntryNames)
		}
		h.AssertNil(t, err)
		allEntryNames = append(allEntryNames, header.Name)
		if header.Name != expected.Name {
			t.Fatalf("expected entry '%d' to have name %q, got %q", i, expected.Name, header.Name)
		}
		if header.Typeflag != expected.Typeflag {
			t.Fatalf("expected entry '%s' to have type %q, got %q", expected.Name, expected.Typeflag, header.Typeflag)
		}
		if expected.Mode != 0 && header.Mode != expected.Mode { // TODO: add modes to all expects to remove the 0 hack
			t.Fatalf("expected entry '%s' to have mode %d, got %d", expected.Name, expected.Mode, header.Mode)
		}
		assertOSSpecificFields(t, expected, header)
		if !header.ModTime.Equal(time.Date(1980, time.January, 1, 0, 0, 1, 0, time.UTC)) {
			t.Fatalf("expected entry '%s' to normalized mod time, got '%s", expected.Name, header.ModTime)
		}
		if header.Uname != "" {
			t.Fatalf("expected entry '%s' to empty Uname, got '%s", expected.Name, header.Uname)
		}
		if header.Gname != "" {
			t.Fatalf("expected entry '%s' to empty Gname, got '%s", expected.Name, header.Gname)
		}
		h.AssertEq(t, header.Typeflag, expected.Typeflag)
	}
	header, err := tr.Next()
	if err != io.EOF {
		t.Fatalf("unexpected archive entry '%s'", header.Name)
	}
}

func parents(t *testing.T, file string) []*tar.Header {
	t.Helper()
	parent := filepath.Dir(file)
	if parent == `/` || parent == filepath.VolumeName(file)+`\` {
		return []*tar.Header{}
	}
	fi, err := os.Stat(parent)
	h.AssertNil(t, err)
	return append(parents(t, parent), parentHeader(parent, fi))
}
