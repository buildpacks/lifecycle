package layers_test

import (
	"archive/tar"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/layers"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestSnapshotLayers(t *testing.T) {
	spec.Run(t, "Factory", testSnapshotLayers, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testSnapshotLayers(t *testing.T, when spec.G, it spec.S) {
	var (
		factory    *layers.Factory
		snapshot   string
		logHandler = memory.New()
	)
	it.Before(func() {
		var err error
		artifactDir, err := ioutil.TempDir("", "layers.slices.layer")
		h.AssertNil(t, err)
		factory = &layers.Factory{
			ArtifactsDir: artifactDir,
			Logger:       &log.Logger{Handler: logHandler},
			UID:          2222,
			GID:          3333,
		}
		snapshot, err = filepath.Abs(filepath.Join("testdata", "example_snapshot.tgz"))
		h.AssertNil(t, err)
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(factory.ArtifactsDir))
	})

	when("#SnapshotLayer", func() {
		var snapshotLayer layers.Layer

		it.Before(func() {
			var err error
			snapshotLayer, err = factory.SnapshotLayer("some-layer-id", snapshot)
			h.AssertNil(t, err)
		})

		it("does something", func() {
			h.AssertEq(t, snapshotLayer.ID, "some-layer-id")
			assertTarEntries(t, snapshotLayer.TarPath, []*tar.Header{
				{
					Name:     tarPath("usr/bin/.wh.apt"),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeReg,
				},
				{
					Name:     tarPath("/"),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeDir,
				},
				{
					Name:     tarPath("bin/"),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeDir,
				},
				{
					Name:     tarPath("bin/exe-to-snapshot"),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeReg,
				},
				{
					Name:     tarPath("cnb/"),
					Uid:      factory.UID,
					Gid:      factory.GID,
					Typeflag: tar.TypeDir,
				},
				{
					Name:     tarPath("file-to-snapshot"),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeReg,
				},
				{
					Name:     tarPath("layers/"),
					Uid:      factory.UID,
					Gid:      factory.GID,
					Typeflag: tar.TypeDir,
				},
				{
					Name:     tarPath("tmp/"),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeDir,
				},
				{
					Name:     tarPath("usr/"),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeDir,
				},
				{
					Name:     tarPath("usr/bin/"),
					Uid:      0,
					Gid:      0,
					Typeflag: tar.TypeDir,
				},
			})
		})
	})
}
