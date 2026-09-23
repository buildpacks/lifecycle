package buildpack_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	llog "github.com/buildpacks/lifecycle/log"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestLayers(t *testing.T) {
	spec.Run(t, "unit-layers", testLayers, spec.Report(report.Terminal{}))
}

func testLayers(t *testing.T, when spec.G, it spec.S) {
	when("#ValidateLayerName", func() {
		it("accepts valid layer names", func() {
			validNames := []string{
				"my-layer",
				"layer1",
				"foo.bar",
				"1000",
				"nonroot",
				"launch",
			}
			for _, name := range validNames {
				h.AssertNil(t, buildpack.ValidateLayerName(name))
			}
		})

		it("rejects invalid layer names", func() {
			invalidNames := []string{
				"",
				".",
				"..",
				"../foo",
				"foo/../bar",
				"foo/bar",
				"/foo",
				"foo/",
				"./foo",
				"foo/.",
				`..\foo`,
				`foo\bar`,
				`\foo`,
				"C:foo",
			}
			for _, name := range invalidNames {
				err := buildpack.ValidateLayerName(name)
				h.AssertError(t, err, "invalid layer name")
			}
		})
	})

	when("#ReadLayersDir", func() {
		it("skips files whose layer name is not a single path segment", func() {
			tmpDir := t.TempDir()
			bpDirPath := filepath.Join(tmpDir, "some-bp")
			h.AssertNil(t, os.MkdirAll(bpDirPath, 0750))
			h.AssertNil(t, os.WriteFile(filepath.Join(bpDirPath, ".toml"), []byte(""), 0600))
			h.AssertNil(t, os.WriteFile(filepath.Join(bpDirPath, "real-layer.toml"), []byte(""), 0600))

			logger := &log.Logger{Handler: memory.New()}
			layersDir, err := buildpack.ReadLayersDir(tmpDir, buildpack.GroupElement{ID: "some-bp", API: "0.9"}, logger)
			h.AssertNil(t, err)

			found := layersDir.FindLayers(func(_ buildpack.Layer) bool { return true })
			h.AssertEq(t, len(found), 1)
			h.AssertEq(t, found[0].Identifier(), "some-bp:real-layer")
		})
	})

	when("#NewLayer", func() {
		var (
			layersDir  buildpack.LayersDir
			logHandler *memory.Handler
			logger     llog.Logger
		)

		it.Before(func() {
			logHandler = memory.New()
			logger = &log.Logger{Handler: logHandler}
			layersDir = buildpack.LayersDir{
				Path:      filepath.Join("some", "layers", "dir"),
				Buildpack: buildpack.GroupElement{ID: "some-bp", API: "0.9"},
			}
		})

		it("creates a layer when name is valid", func() {
			layer := layersDir.NewLayer("valid-layer", "0.9", logger)
			h.AssertNotNil(t, layer)
			h.AssertEq(t, layer.Path(), filepath.Join("some", "layers", "dir", "valid-layer"))
			h.AssertEq(t, layer.Identifier(), "some-bp:valid-layer")
			h.AssertEq(t, len(logHandler.Entries), 0)
		})

		it("returns nil and logs a warning when name is invalid", func() {
			layer := layersDir.NewLayer("../invalid-layer", "0.9", logger)
			h.AssertNil(t, layer)
			h.AssertEq(t, len(logHandler.Entries), 1)
			h.AssertEq(t, logHandler.Entries[0].Level, log.WarnLevel)
			h.AssertStringContains(t, logHandler.Entries[0].Message, "invalid layer name")
		})

		it("handles nil logger gracefully when name is invalid", func() {
			layer := layersDir.NewLayer("../invalid-layer", "0.9", nil)
			h.AssertNil(t, layer)
		})
	})

	when("#eachLayer", func() {
		it("ignores a .toml file with no layer name and still visits real layers", func() {
			bpLayersDir := t.TempDir()
			h.AssertNil(t, os.WriteFile(filepath.Join(bpLayersDir, ".toml"), []byte(""), 0600))
			h.AssertNil(t, os.WriteFile(filepath.Join(bpLayersDir, "real-layer.toml"), []byte(""), 0600))

			var visited []string
			h.AssertNil(t, buildpack.EachLayer(bpLayersDir, nil, func(layerPath string) error {
				visited = append(visited, layerPath)
				return nil
			}))

			h.AssertEq(t, visited, []string{filepath.Join(bpLayersDir, "real-layer")})
		})
	})
}
