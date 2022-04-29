package platform_test

import (
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDirStore(t *testing.T) {
	spec.Run(t, "DirStore", testDirStore, spec.Report(report.Terminal{}))
}

func testDirStore(t *testing.T, when spec.G, it spec.S) {
	var dirStore *platform.DirStore

	it.Before(func() {
		var err error
		dirStore, err = platform.NewDirStore(
			filepath.Join("testdata", "cnb", "buildpacks"),
			filepath.Join("testdata", "cnb", "extensions"),
		)
		h.AssertNil(t, err)
	})

	when(".LookupBp", func() {
		it("returns buildpack from buildpack.toml", func() {
			bp, err := dirStore.LookupBp("A", "v1")
			h.AssertNil(t, err)

			config := bp.ConfigFile()
			h.AssertEq(t, config.Buildpack.ID, "A")
			h.AssertEq(t, config.Buildpack.Name, "Buildpack A")
			h.AssertEq(t, config.Buildpack.Version, "v1")
			h.AssertEq(t, config.Buildpack.Homepage, "Buildpack A Homepage")
			h.AssertEq(t, config.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
		})
	})

	when(".LookupExt", func() {
		it("returns extension from extension.toml", func() {
			ext, err := dirStore.LookupExt("A", "v1")
			h.AssertNil(t, err)

			// TODO: validate config in buildpack package
			config := ext.ConfigFile()
			h.AssertEq(t, config.Extension.ID, "A")
			h.AssertEq(t, config.Extension.Name, "Extension A")
			h.AssertEq(t, config.Extension.Version, "v1")
			h.AssertEq(t, config.Extension.Homepage, "Extension A Homepage")
		})
	})
}
