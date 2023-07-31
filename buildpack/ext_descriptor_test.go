package buildpack_test

import (
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExtDescriptor(t *testing.T) {
	spec.Run(t, "ExtDescriptor", testExtDescriptor, spec.Report(report.Terminal{}))
}

func testExtDescriptor(t *testing.T, when spec.G, it spec.S) {
	when("#ReadExtDescriptor", func() {
		it("returns an extension descriptor", func() {
			path := filepath.Join("testdata", "extension", "by-id", "A", "v1", "extension.toml")
			descriptor, err := buildpack.ReadExtDescriptor(path)
			h.AssertNil(t, err)

			h.AssertEq(t, descriptor.WithAPI, "0.9")
			h.AssertEq(t, descriptor.Extension.ID, "A")
			h.AssertEq(t, descriptor.Extension.Name, "Extension A")
			h.AssertEq(t, descriptor.Extension.Version, "v1")
			h.AssertEq(t, descriptor.Extension.Homepage, "Extension A Homepage")
			h.AssertEq(t, len(descriptor.Targets), 1)
			h.AssertEq(t, descriptor.Targets[0].OS, "linux")
			h.AssertEq(t, descriptor.Targets[0].Arch, "")
		})
		it("infers */* if there's no files to infer from", func() {
			path := filepath.Join("testdata", "extension", "by-id", "B", "v1", "extension.toml")
			descriptor, err := buildpack.ReadExtDescriptor(path)
			h.AssertNil(t, err)
			h.AssertEq(t, len(descriptor.Targets), 1)
			h.AssertEq(t, descriptor.Targets[0].OS, "")
			h.AssertEq(t, descriptor.Targets[0].Arch, "")
		})
		it("slices, it dices, it even does windows", func() {
			path := filepath.Join("testdata", "extension", "by-id", "D", "v1", "extension.toml")
			descriptor, err := buildpack.ReadExtDescriptor(path)
			h.AssertNil(t, err)
			h.AssertEq(t, len(descriptor.Targets), 1)
			h.AssertEq(t, descriptor.Targets[0].OS, "windows")
			h.AssertEq(t, descriptor.Targets[0].Arch, "")
		})
	})
}
