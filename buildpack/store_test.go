package buildpack_test

import (
	"path/filepath"
	"testing"

	"github.com/buildpacks/lifecycle/buildpack"
	h "github.com/buildpacks/lifecycle/testhelpers"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestStore(t *testing.T) {
	spec.Run(t, "BuildableStore", testStore, spec.Report(report.Terminal{}))
}

func testStore(t *testing.T, when spec.G, it spec.S) {
	var store *buildpack.BpStore

	it.Before(func() {
		var err error
		store, err = buildpack.NewBuildpackStore(filepath.Join("testdata", "by-id"))
		h.AssertNil(t, err)
	})

	when("Lookup", func() {
		it("returns buildpack from buildpack.toml", func() {
			bp, err := store.Lookup("A", "v1")
			h.AssertNil(t, err)

			config := bp.ConfigFile()
			h.AssertEq(t, config.Buildpack.ID, "A")
			h.AssertEq(t, config.Buildpack.Name, "Buildpack A")
			h.AssertEq(t, config.Buildpack.Version, "v1")
			h.AssertEq(t, config.Buildpack.Homepage, "Buildpack A Homepage")
			h.AssertEq(t, config.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
		})
	})
}
