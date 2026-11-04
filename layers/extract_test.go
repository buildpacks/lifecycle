package layers_test

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/layers"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExtract(t *testing.T) {
	spec.Run(t, "extract", testLayersExtract, spec.Report(report.Terminal{}))
}

func testLayersExtract(t *testing.T, when spec.G, it spec.S) {
	var layersDir, artifactsDir string

	it.Before(func() {
		var err error
		layersDir, err = os.MkdirTemp("", "lifecycle.extract.layers.")
		h.AssertNil(t, err)
		artifactsDir, err = os.MkdirTemp("", "lifecycle.extract.artifacts.")
		h.AssertNil(t, err)
	})

	it.After(func() {
		_ = os.RemoveAll(layersDir)
		_ = os.RemoveAll(artifactsDir)
	})

	when("#ExtractConfined", func() {
		it("extracts a layer whose entries live under the root (including parent dirs)", func() {
			src := filepath.Join(layersDir, "sbom", "launch")
			h.Mkdir(t, src)
			h.Mkfile(t, "bom-data", filepath.Join(src, "some-file"))

			factory := &layers.Factory{ArtifactsDir: artifactsDir}
			layer, err := factory.DirLayer("launch.sbom", src, "")
			h.AssertNil(t, err)

			h.AssertNil(t, os.RemoveAll(filepath.Join(layersDir, "sbom")))

			rc, err := os.Open(layer.TarPath)
			h.AssertNil(t, err)
			defer func() { _ = rc.Close() }()

			h.AssertNil(t, layers.ExtractConfined(rc, layersDir))

			got := h.MustReadFile(t, filepath.Join(layersDir, "sbom", "launch", "some-file"))
			h.AssertEq(t, string(got), "bom-data")
		})

		it("rejects an entry that escapes the confinement root", func() {
			escapeTarget := filepath.Join(filepath.Dir(layersDir), "lifecycle-escaped-file")

			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			h.AssertNil(t, tw.WriteHeader(&tar.Header{
				Name:     escapeTarget,
				Typeflag: tar.TypeReg,
				Mode:     0644,
				Size:     0,
			}))
			h.AssertNil(t, tw.Close())

			h.AssertError(t, layers.ExtractConfined(&buf, layersDir), "escapes destination root")

			_, statErr := os.Lstat(escapeTarget)
			h.AssertError(t, statErr, "no such file or directory")
		})
	})

	when("#Extract", func() {
		var destDir string

		it.Before(func() {
			var err error
			destDir, err = os.MkdirTemp("", "lifecycle.extract.dest.")
			h.AssertNil(t, err)
		})

		it.After(func() {
			_ = os.RemoveAll(destDir)
		})

		it("extracts the layer entries relative to the destination directory", func() {
			src := filepath.Join(layersDir, "sbom", "launch")
			h.Mkdir(t, src)
			h.Mkfile(t, "bom-data", filepath.Join(src, "some-file"))

			factory := &layers.Factory{ArtifactsDir: artifactsDir}
			layer, err := factory.DirLayer("launch.sbom", src, "")
			h.AssertNil(t, err)

			rc, err := os.Open(layer.TarPath)
			h.AssertNil(t, err)
			defer func() { _ = rc.Close() }()

			h.AssertNil(t, layers.Extract(rc, destDir))

			got := h.MustReadFile(t, filepath.Join(destDir, src, "some-file"))
			h.AssertEq(t, string(got), "bom-data")
		})

		it("rejects an entry that escapes the destination directory", func() {
			var buf bytes.Buffer
			tw := tar.NewWriter(&buf)
			h.AssertNil(t, tw.WriteHeader(&tar.Header{
				Name:     "../lifecycle-escaped-file",
				Typeflag: tar.TypeReg,
				Mode:     0644,
				Size:     0,
			}))
			h.AssertNil(t, tw.Close())

			h.AssertError(t, layers.Extract(&buf, destDir), "escapes destination root")

			_, statErr := os.Lstat(filepath.Join(filepath.Dir(destDir), "lifecycle-escaped-file"))
			h.AssertError(t, statErr, "no such file or directory")
		})
	})
}
