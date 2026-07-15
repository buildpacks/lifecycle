package buildpack_test

import (
	"path/filepath"
	"testing"

	"github.com/buildpacks/lifecycle/buildpack"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExtDescriptor(t *testing.T) {
	t.Run("#ReadExtDescriptor", func(t *testing.T) {
		t.Run("returns an extension descriptor", func(t *testing.T) {
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
		t.Run("infers */* if there's no files to infer from", func(t *testing.T) {
			path := filepath.Join("testdata", "extension", "by-id", "B", "v1", "extension.toml")
			descriptor, err := buildpack.ReadExtDescriptor(path)
			h.AssertNil(t, err)
			h.AssertEq(t, len(descriptor.Targets), 1)
			h.AssertEq(t, descriptor.Targets[0].OS, "")
			h.AssertEq(t, descriptor.Targets[0].Arch, "")
		})
	})
}
