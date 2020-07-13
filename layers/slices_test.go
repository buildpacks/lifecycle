package layers_test

import (
	"archive/tar"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/layers"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestSlices(t *testing.T) {
	spec.Run(t, "Factory", testSlices, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testSlices(t *testing.T, when spec.G, it spec.S) {
	var (
		slicer     *layers.Factory
		dirToSlice string
	)
	it.Before(func() {
		var err error
		artifactDir, err := ioutil.TempDir("", "layers.slices.layer")
		h.AssertNil(t, err)
		slicer = &layers.Factory{
			ArtifactsDir: artifactDir,
			UID:          1234,
			GID:          4321,
		}
		dirToSlice, err = filepath.Abs(filepath.Join("testdata", "slices", "dir-to-slice"))
		h.AssertNil(t, err)
	})

	it.After(func() {
		os.RemoveAll(slicer.ArtifactsDir)
	})

	when("#SliceLayers", func() {
		when("there are no slices", func() {
			it("creates a single app layer", func() {
				sliceLayers, err := slicer.SliceLayers(dirToSlice, []layers.Slice{})
				h.AssertNil(t, err)
				h.AssertEq(t, len(sliceLayers), 1)
				assertTarEntries(t, sliceLayers[0].TarPath, append(dirAndParents(dirToSlice), []entry{
					{name: filepath.Join(dirToSlice, "dir-link"), typeFlag: tar.TypeSymlink},
					{name: filepath.Join(dirToSlice, "file-link.txt"), typeFlag: tar.TypeSymlink},
					{name: filepath.Join(dirToSlice, "file.txt"), typeFlag: tar.TypeReg},
					{name: filepath.Join(dirToSlice, "other-dir"), typeFlag: tar.TypeDir},
					{name: filepath.Join(dirToSlice, "other-dir", "other-file.md"), typeFlag: tar.TypeReg},
					{name: filepath.Join(dirToSlice, "other-dir", "other-file.txt"), typeFlag: tar.TypeReg},
					{name: filepath.Join(dirToSlice, "some-dir"), typeFlag: tar.TypeDir},
					{name: filepath.Join(dirToSlice, "some-dir", "file.md"), typeFlag: tar.TypeReg},
					{name: filepath.Join(dirToSlice, "some-dir", "some-file.txt"), typeFlag: tar.TypeReg},
				}...))
			})

			it("resolves relative paths", func() {
				sliceLayers, err := slicer.SliceLayers(filepath.Join("testdata", "slices", "dir-to-slice"), []layers.Slice{})
				h.AssertNil(t, err)
				h.AssertEq(t, len(sliceLayers), 1)
				assertTarEntries(t, sliceLayers[0].TarPath, append(dirAndParents(dirToSlice), []entry{
					{name: filepath.Join(dirToSlice, "dir-link"), typeFlag: tar.TypeSymlink},
					{name: filepath.Join(dirToSlice, "file-link.txt"), typeFlag: tar.TypeSymlink},
					{name: filepath.Join(dirToSlice, "file.txt"), typeFlag: tar.TypeReg},
					{name: filepath.Join(dirToSlice, "other-dir"), typeFlag: tar.TypeDir},
					{name: filepath.Join(dirToSlice, "other-dir", "other-file.md"), typeFlag: tar.TypeReg},
					{name: filepath.Join(dirToSlice, "other-dir", "other-file.txt"), typeFlag: tar.TypeReg},
					{name: filepath.Join(dirToSlice, "some-dir"), typeFlag: tar.TypeDir},
					{name: filepath.Join(dirToSlice, "some-dir", "file.md"), typeFlag: tar.TypeReg},
					{name: filepath.Join(dirToSlice, "some-dir", "some-file.txt"), typeFlag: tar.TypeReg},
				}...))
			})
		})

		when("there are n slices", func() {
			var sliceLayers []layers.Layer

			it.Before(func() {
				var err error
				sliceLayers, err = slicer.SliceLayers(dirToSlice, []layers.Slice{
					{Paths: []string{"*.txt", "**/*.txt"}},
					{Paths: []string{"other-dir"}},
					{Paths: []string{"dir-link/*"}},
					{Paths: []string{"../**/dir-to-exclude"}},
				})
				h.AssertNil(t, err)
			})

			it("creates n+1 layers", func() {
				h.AssertEq(t, len(sliceLayers), 5)
			})

			it("creates slice from pattern", func() {
				assertTarEntries(t, sliceLayers[0].TarPath, append(dirAndParents(dirToSlice), []entry{
					{name: filepath.Join(dirToSlice, "file-link.txt"), typeFlag: tar.TypeSymlink},
					{name: filepath.Join(dirToSlice, "file.txt"), typeFlag: tar.TypeReg},
					{name: filepath.Join(dirToSlice, "other-dir"), typeFlag: tar.TypeDir},
					{name: filepath.Join(dirToSlice, "other-dir", "other-file.txt"), typeFlag: tar.TypeReg},
					{name: filepath.Join(dirToSlice, "some-dir"), typeFlag: tar.TypeDir},
					{name: filepath.Join(dirToSlice, "some-dir", "some-file.txt"), typeFlag: tar.TypeReg},
				}...))
			})

			it("accepts dirs", func() {
				assertTarEntries(t, sliceLayers[1].TarPath, append(dirAndParents(dirToSlice), []entry{
					{name: filepath.Join(dirToSlice, "other-dir"), typeFlag: tar.TypeDir},
					{name: filepath.Join(dirToSlice, "other-dir", "other-file.md"), typeFlag: tar.TypeReg},
				}...))
			})

			it("doesn't glob through symlinks", func() {
				assertTarEntries(t, sliceLayers[2].TarPath, []entry{})
			})

			it("doesn't glob files outside of dir", func() {
				assertTarEntries(t, sliceLayers[3].TarPath, []entry{})
			})

			it("creates a layer with the remaining files", func() {
				assertTarEntries(t, sliceLayers[4].TarPath, append(dirAndParents(dirToSlice), []entry{
					{name: filepath.Join(dirToSlice, "dir-link"), typeFlag: tar.TypeSymlink},
					{name: filepath.Join(dirToSlice, "some-dir"), typeFlag: tar.TypeDir},
					{name: filepath.Join(dirToSlice, "some-dir", "file.md"), typeFlag: tar.TypeReg},
				}...))
			})
		})
	})
}

type entry struct {
	name     string
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
		h.AssertEq(t, header.Typeflag, expected.typeFlag)
	}
	header, err := tr.Next()
	if err != io.EOF {
		t.Fatalf("unexpected archive entry '%s'", header.Name)
	}
}

func dirAndParents(file string) []entry {
	parent := filepath.Dir(file)
	fileEntry := entry{name: file, typeFlag: tar.TypeDir}
	if parent == "/" {
		return []entry{fileEntry}
	}
	return append(dirAndParents(parent), fileEntry)
}
