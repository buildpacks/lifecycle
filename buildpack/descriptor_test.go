package buildpack_test

import (
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDescriptor(t *testing.T) {
	spec.Run(t, "Descriptor", testDescriptor, spec.Report(report.Terminal{}))
}

func testDescriptor(t *testing.T, when spec.G, it spec.S) {
	when("#ReadDescriptor", func() {
		when("buildpack", func() {
			it("returns a descriptor with buildpack info", func() {
				path := filepath.Join("testdata", "by-id", "A", "v1", "buildpack.toml")
				descriptor, err := buildpack.ReadDescriptor(path)
				h.AssertNil(t, err)

				h.AssertEq(t, descriptor.API, "0.7")
				h.AssertEq(t, descriptor.Buildpack.ID, "A")
				h.AssertEq(t, descriptor.Buildpack.Name, "Buildpack A")
				h.AssertEq(t, descriptor.Buildpack.Version, "v1")
				h.AssertEq(t, descriptor.Buildpack.Homepage, "Buildpack A Homepage")
				h.AssertEq(t, descriptor.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
			})
		})

		when("extension", func() {
			it("returns a descriptor with extension info", func() {
				path := filepath.Join("testdata", "by-id", "extA", "v1", "extension.toml")
				descriptor, err := buildpack.ReadDescriptor(path)
				h.AssertNil(t, err)

				h.AssertEq(t, descriptor.API, "0.9")
				h.AssertEq(t, descriptor.Extension.ID, "A")
				h.AssertEq(t, descriptor.Extension.Name, "Extension A")
				h.AssertEq(t, descriptor.Extension.Version, "v1")
				h.AssertEq(t, descriptor.Extension.Homepage, "Extension A Homepage")
			})
		})
	})

	when(".Info", func() {
		var info = &buildpack.Info{
			ID:       "A",
			Version:  "v1",
			ClearEnv: true,
		}

		when("buildpack", func() {
			it("returns buildpack info", func() {
				descriptor := buildpack.Descriptor{
					API:       "0.9",
					Buildpack: *info,
				}

				h.AssertEq(t, descriptor.Info(), info)
			})
		})

		when("extension", func() {
			it("returns extension info", func() {
				descriptor := buildpack.Descriptor{
					API:       "0.9",
					Extension: *info,
				}

				h.AssertEq(t, descriptor.Info(), info)
			})
		})
	})

	when(".ToGroupElement", func() {
		when("buildpack", func() {
			it("returns a group element that is a buildpack", func() {
				descriptor := buildpack.Descriptor{
					API: "0.9",
					Buildpack: buildpack.Info{
						ID:      "A",
						Version: "v1",
					},
				}
				groupEl := descriptor.ToGroupElement(false)

				h.AssertEq(t, groupEl.Extension, false)
			})
		})

		when("extension", func() {
			it("returns a group element that is an extension", func() {
				descriptor := buildpack.Descriptor{
					API: "0.9",
					Extension: buildpack.Info{
						ID:      "A",
						Version: "v1",
					},
				}
				groupEl := descriptor.ToGroupElement(false)

				h.AssertEq(t, groupEl.Extension, true)
				h.AssertEq(t, groupEl.Optional, true)
			})
		})
	})
}
