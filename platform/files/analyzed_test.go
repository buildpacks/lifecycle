package files_test

import (
	"os"
	"testing"

	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/internal/encoding"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestAnalyzed(t *testing.T) {
	spec.Run(t, "Analyzed", testAnalyzed)
}

func testAnalyzed(t *testing.T, when spec.G, it spec.S) {
	when("analyzed.toml", func() {
		when("it is old it stays old", func() {
			it("serializes and deserializes", func() {
				amd := files.Analyzed{
					PreviousImage: &files.ImageIdentifier{Reference: "previous-img"},
					LayersMetadata: files.LayersMetadata{
						Stack: &files.Stack{
							RunImage: files.RunImageForExport{Image: "imagine that"},
						},
					},
					RunImage: &files.RunImage{Reference: "some-ref"},
				}
				f := h.TempFile(t, "", "")
				h.AssertNil(t, encoding.WriteTOML(f, amd))
				amd2, err := files.ReadAnalyzed(f, nil)
				h.AssertNil(t, err)
				h.AssertEq(t, amd.PreviousImageRef(), amd2.PreviousImageRef())
				h.AssertEq(t, amd.LayersMetadata, amd2.LayersMetadata)
				h.AssertEq(t, amd.BuildImage, amd2.BuildImage)
			})

			it("serializes to the old format", func() {
				amd := files.Analyzed{
					PreviousImage: &files.ImageIdentifier{Reference: "previous-img"},
					LayersMetadata: files.LayersMetadata{
						Stack: &files.Stack{
							RunImage: files.RunImageForExport{Image: "imagine that"},
						},
					},
					RunImage: &files.RunImage{Reference: "some-ref"},
				}
				f := h.TempFile(t, "", "")
				h.AssertNil(t, encoding.WriteTOML(f, amd))
				contents, err := os.ReadFile(f)
				h.AssertNil(t, err)
				expectedContents := `[image]
  reference = "previous-img"

[metadata]
  [metadata.config]
    sha = ""
  [metadata.launcher]
    sha = ""
  [metadata.process-types]
    sha = ""
  [metadata.run-image]
    top-layer = ""
    reference = ""
  [metadata.stack]
    [metadata.stack.run-image]
      image = "imagine that"

[run-image]
  reference = "some-ref"
`
				h.AssertEq(t, string(contents), expectedContents)
			})
		})

		when("it is new it stays new", func() {
			it("serializes and deserializes", func() {
				amd := files.Analyzed{
					PreviousImage: &files.ImageIdentifier{Reference: "the image formerly known as prince"},
					RunImage: &files.RunImage{
						Reference:      "librarian",
						TargetMetadata: &files.TargetMetadata{OS: "os/2 warp", Arch: "486"},
					},
					BuildImage: &files.ImageIdentifier{Reference: "implementation"},
				}
				f := h.TempFile(t, "", "")
				h.AssertNil(t, encoding.WriteTOML(f, amd))
				amd2, err := files.ReadAnalyzed(f, nil)
				h.AssertNil(t, err)
				h.AssertEq(t, amd.PreviousImageRef(), amd2.PreviousImageRef())
				h.AssertEq(t, amd.LayersMetadata, amd2.LayersMetadata)
				h.AssertEq(t, amd.BuildImage, amd2.BuildImage)
			})
		})
	})
}
