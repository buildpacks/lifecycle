package buildpack_test

import (
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestBpDescriptor(t *testing.T) {
	spec.Run(t, "BpDescriptor", testBpDescriptor, spec.Report(report.Terminal{}))
}

func testBpDescriptor(t *testing.T, when spec.G, it spec.S) {
	when("#ReadBpDescriptor", func() {
		it("returns a buildpack descriptor", func() {
			path := filepath.Join("testdata", "buildpack", "by-id", "A", "v1", "buildpack.toml")
			descriptor, err := buildpack.ReadBpDescriptor(path)
			h.AssertNil(t, err)

			h.AssertEq(t, descriptor.WithAPI, "0.7")
			h.AssertEq(t, descriptor.Buildpack.ID, "A")
			h.AssertEq(t, descriptor.Buildpack.Name, "Buildpack A")
			h.AssertEq(t, descriptor.Buildpack.Version, "v1")
			h.AssertEq(t, descriptor.Buildpack.Homepage, "Buildpack A Homepage")
			h.AssertEq(t, descriptor.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
		})
	})
}
