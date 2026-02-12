package patch_test

import (
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/patch"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestPatcher(t *testing.T) {
	spec.Run(t, "Patcher", testPatcher, spec.Parallel(), spec.Report(report.Terminal{}))
}

func testPatcher(t *testing.T, when spec.G, it spec.S) {
	var (
		patcher    *patch.LayerPatcher
		logHandler *memory.Handler
	)

	it.Before(func() {
		logHandler = memory.New()
		patcher = patch.NewLayerPatcher(&log.Logger{Handler: logHandler}, nil, nil, false, nil)
	})

	when("ApplyPatches", func() {
		when("no patches provided", func() {
			it("returns nil without error", func() {
				metadata := &files.LayersMetadataCompat{}
				patches := files.LayerPatchesFile{}

				results, cleanup, err := patcher.ApplyPatches(nil, metadata, patches, "linux", "amd64", "")
				h.AssertNil(t, err)
				h.AssertEq(t, len(results), 0)
				if cleanup != nil {
					cleanup()
				}
			})
		})

		when("patch references non-existent layer", func() {
			it("skips the patch without error", func() {
				metadata := &files.LayersMetadataCompat{
					Buildpacks: []buildpack.LayersMetadata{
						{
							ID: "buildpack-a",
							Layers: map[string]buildpack.LayerMetadata{
								"layer1": {SHA: "sha1"},
							},
						},
					},
				}
				patches := files.LayerPatchesFile{
					Patches: []files.LayerPatch{
						{
							Buildpack:  "buildpack-nonexistent",
							Layer:      "layer1",
							PatchImage: "patch-image:latest",
						},
					},
				}

				results, cleanup, err := patcher.ApplyPatches(nil, metadata, patches, "linux", "amd64", "")
				h.AssertNil(t, err)
				h.AssertEq(t, len(results), 0)
				if cleanup != nil {
					cleanup()
				}
			})
		})
	})
}
