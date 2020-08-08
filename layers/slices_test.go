package layers_test

import (
	"archive/tar"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/layers"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestSliceLayers(t *testing.T) {
	spec.Run(t, "Factory", testSlices, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testSlices(t *testing.T, when spec.G, it spec.S) {
	var (
		factory    *layers.Factory
		dirToSlice string
	)
	it.Before(func() {
		var err error
		artifactDir, err := ioutil.TempDir("", "layers.slices.layer")
		h.AssertNil(t, err)
		factory = &layers.Factory{
			ArtifactsDir: artifactDir,
			UID:          1234,
			GID:          4321,
		}
		dirToSlice, err = filepath.Abs(filepath.Join("testdata", "target-dir"))
		h.AssertNil(t, err)
	})

	it.After(func() {
		os.RemoveAll(factory.ArtifactsDir)
	})

	when("#SliceLayers", func() {
		when("there are no slices", func() {
			it("creates a single app layer", func() {
				sliceLayers, err := factory.SliceLayers(dirToSlice, []layers.Slice{})
				h.AssertNil(t, err)
				h.AssertEq(t, len(sliceLayers), 1)
				h.AssertEq(t, sliceLayers[0].ID, "slice-1")
				// parent layers should have uid/gid matching the filesystem
				// the sliced dir and it's children should have normalized uid/gid
				assertTarEntries(t, sliceLayers[0].TarPath, append(parents(t, dirToSlice), []*tar.Header{
					{
						Name:     tarPath(dirToSlice),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeDir,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "dir-link")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeSymlink,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "file-link.txt")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeSymlink,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "file.txt")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "other-dir")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeDir,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "other-dir", "other-file.md")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "other-dir", "other-file.txt")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "some-dir")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeDir,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "some-dir", "file.md")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "some-dir", "some-file.txt")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
				}...))
			})

			it("resolves relative paths", func() {
				sliceLayers, err := factory.SliceLayers(filepath.Join("testdata", "target-dir"), []layers.Slice{})
				h.AssertNil(t, err)
				h.AssertEq(t, len(sliceLayers), 1)
				h.AssertEq(t, sliceLayers[0].ID, "slice-1")
				assertTarEntries(t, sliceLayers[0].TarPath, append(parents(t, dirToSlice), []*tar.Header{
					{
						Name:     tarPath(dirToSlice),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeDir,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "dir-link")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeSymlink,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "file-link.txt")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeSymlink,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "file.txt")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "other-dir")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeDir,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "other-dir", "other-file.md")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "other-dir", "other-file.txt")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "some-dir")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeDir,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "some-dir", "file.md")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "some-dir", "some-file.txt")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
				}...))
			})
		})

		when("there are n slices", func() {
			var sliceLayers []layers.Layer

			it.Before(func() {
				var err error
				sliceLayers, err = factory.SliceLayers(dirToSlice, []layers.Slice{
					{Paths: []string{"*.txt", "**/*.txt"}},
					{Paths: []string{"other-dir"}},
					{Paths: []string{"dir-link/*"}},
					{Paths: []string{"../**/dir-to-exclude"}},
				})
				h.AssertNil(t, err)
			})

			it("creates n+1 layers", func() {
				h.AssertEq(t, len(sliceLayers), 5)
				h.AssertEq(t, sliceLayers[0].ID, "slice-1")
				h.AssertEq(t, sliceLayers[1].ID, "slice-2")
				h.AssertEq(t, sliceLayers[2].ID, "slice-3")
				h.AssertEq(t, sliceLayers[3].ID, "slice-4")
				h.AssertEq(t, sliceLayers[4].ID, "slice-5")
			})

			it("creates slice from pattern", func() {
				assertTarEntries(t, sliceLayers[0].TarPath, append(parents(t, dirToSlice), []*tar.Header{
					{
						Name:     tarPath(dirToSlice),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeDir,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "file-link.txt")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeSymlink,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "file.txt")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "other-dir")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeDir,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "other-dir", "other-file.txt")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "some-dir")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeDir},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "some-dir", "some-file.txt")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
				}...))
			})

			it("accepts dirs", func() {
				assertTarEntries(t, sliceLayers[1].TarPath, append(parents(t, dirToSlice), []*tar.Header{
					{
						Name:     tarPath(dirToSlice),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeDir,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "other-dir")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeDir,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "other-dir", "other-file.md")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
				}...))
			})

			it("doesn't glob through symlinks", func() {
				assertTarEntries(t, sliceLayers[2].TarPath, []*tar.Header{})
			})

			it("doesn't glob files outside of dir", func() {
				assertTarEntries(t, sliceLayers[3].TarPath, []*tar.Header{})
			})

			it("creates a layer with the remaining files", func() {
				assertTarEntries(t, sliceLayers[4].TarPath, append(parents(t, dirToSlice), []*tar.Header{
					{
						Name:     tarPath(dirToSlice),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeDir,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "dir-link")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeSymlink,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "some-dir")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeDir,
					},
					{
						Name:     tarPath(filepath.Join(dirToSlice, "some-dir", "file.md")),
						Uid:      factory.UID,
						Gid:      factory.GID,
						Typeflag: tar.TypeReg,
					},
				}...))
			})
		})
	})
}
