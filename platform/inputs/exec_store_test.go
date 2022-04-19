package inputs_test

import (
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/platform/inputs"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExecStore(t *testing.T) {
	spec.Run(t, "ExecStore", testExecStore, spec.Report(report.Terminal{}))
}

func testExecStore(t *testing.T, when spec.G, it spec.S) {
	var execStore *inputs.ExecStore

	it.Before(func() {
		var err error
		execStore, err = inputs.NewExecStore(filepath.Join("testdata", "buildpacks"), filepath.Join("testdata", "extensions"))
		h.AssertNil(t, err)
	})

	when("LookupBp", func() {
		it("returns buildpack from buildpack.toml", func() {
			bp, err := execStore.LookupBp("A", "v1")
			h.AssertNil(t, err)

			// TODO: validate config
			config := bp.ConfigFile()
			h.AssertEq(t, config.Buildpack.ID, "A")
			h.AssertEq(t, config.Buildpack.Name, "Buildpack A")
			h.AssertEq(t, config.Buildpack.Version, "v1")
			h.AssertEq(t, config.Buildpack.Homepage, "Buildpack A Homepage")
			h.AssertEq(t, config.Buildpack.SBOM, []string{"application/vnd.cyclonedx+json"})
		})
	})

	when("LookupBp", func() {
		it("returns extension from extension.toml", func() {
			ext, err := execStore.LookupExt("A", "v1")
			h.AssertNil(t, err)

			// TODO: validate config
			config := ext.ConfigFile()
			h.AssertEq(t, config.Extension.ID, "A")
			h.AssertEq(t, config.Extension.Name, "Extension A")
			h.AssertEq(t, config.Extension.Version, "v1")
			h.AssertEq(t, config.Extension.Homepage, "Extension A Homepage")
		})
	})
}
