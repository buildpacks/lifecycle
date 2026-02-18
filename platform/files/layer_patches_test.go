package files_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestLayerPatches(t *testing.T) {
	spec.Run(t, "LayerPatches", testLayerPatches, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testLayerPatches(t *testing.T, when spec.G, it spec.S) {
	var (
		tmpDir string
	)

	it.Before(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "layer-patches-test")
		h.AssertNil(t, err)
	})

	it.After(func() {
		_ = os.RemoveAll(tmpDir)
	})

	when("ReadLayerPatches", func() {
		when("file exists with valid JSON", func() {
			it("parses the patches file", func() {
				content := `{
					"patches": [
						{
							"buildpack": "org/buildpack-java",
							"layer": "jre",
							"data": {"artifact.version": "17.*"},
							"patch-image": "registry.example.com/patches/java:17",
							"patch-image.mirrors": ["mirror.example.com/patches/java:17"]
						}
					]
				}`
				path := filepath.Join(tmpDir, "patches.json")
				h.AssertNil(t, os.WriteFile(path, []byte(content), 0600))

				patches, err := files.Handler.ReadLayerPatches(path)
				h.AssertNil(t, err)

				h.AssertEq(t, len(patches.Patches), 1)
				h.AssertEq(t, patches.Patches[0].Buildpack, "org/buildpack-java")
				h.AssertEq(t, patches.Patches[0].Layer, "jre")
				h.AssertEq(t, patches.Patches[0].Data["artifact.version"], "17.*")
				h.AssertEq(t, patches.Patches[0].PatchImage, "registry.example.com/patches/java:17")
				h.AssertEq(t, len(patches.Patches[0].PatchImageMirrors), 1)
				h.AssertEq(t, patches.Patches[0].PatchImageMirrors[0], "mirror.example.com/patches/java:17")
			})
		})

		when("file exists with multiple patches", func() {
			it("parses all patches", func() {
				content := `{
					"patches": [
						{
							"buildpack": "buildpack-a",
							"layer": "layer1",
							"patch-image": "image1"
						},
						{
							"buildpack": "buildpack-b",
							"layer": "layer2",
							"patch-image": "image2"
						}
					]
				}`
				path := filepath.Join(tmpDir, "patches.json")
				h.AssertNil(t, os.WriteFile(path, []byte(content), 0600))

				patches, err := files.Handler.ReadLayerPatches(path)
				h.AssertNil(t, err)

				h.AssertEq(t, len(patches.Patches), 2)
			})
		})

		when("file exists with empty patches", func() {
			it("returns empty patches", func() {
				content := `{"patches": []}`
				path := filepath.Join(tmpDir, "patches.json")
				h.AssertNil(t, os.WriteFile(path, []byte(content), 0600))

				patches, err := files.Handler.ReadLayerPatches(path)
				h.AssertNil(t, err)

				h.AssertEq(t, len(patches.Patches), 0)
			})
		})

		when("file does not exist", func() {
			it("returns an error", func() {
				path := filepath.Join(tmpDir, "nonexistent.json")

				_, err := files.Handler.ReadLayerPatches(path)

				h.AssertNotNil(t, err)
				h.AssertStringContains(t, err.Error(), "layer patches file not found")
			})
		})

		when("file contains invalid JSON", func() {
			it("returns an error", func() {
				content := `{"patches": invalid}`
				path := filepath.Join(tmpDir, "patches.json")
				h.AssertNil(t, os.WriteFile(path, []byte(content), 0600))

				_, err := files.Handler.ReadLayerPatches(path)

				h.AssertNotNil(t, err)
				h.AssertStringContains(t, err.Error(), "failed to parse layer patches file")
			})
		})

		when("file contains patch without data", func() {
			it("parses successfully with nil data", func() {
				content := `{
					"patches": [
						{
							"buildpack": "buildpack-a",
							"layer": "layer1",
							"patch-image": "image1"
						}
					]
				}`
				path := filepath.Join(tmpDir, "patches.json")
				h.AssertNil(t, os.WriteFile(path, []byte(content), 0600))

				patches, err := files.Handler.ReadLayerPatches(path)
				h.AssertNil(t, err)

				h.AssertEq(t, len(patches.Patches), 1)
				h.AssertEq(t, len(patches.Patches[0].Data), 0)
			})
		})
	})
}
