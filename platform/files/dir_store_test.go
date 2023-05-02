package files_test

import (
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDirStore(t *testing.T) {
	spec.Run(t, "DirStore", testDirStore, spec.Report(report.Terminal{}))
}

func testDirStore(t *testing.T, when spec.G, it spec.S) {
	var dirStore *files.DirStore

	it.Before(func() {
		var err error
		dirStore = files.NewDirStore(
			filepath.Join("testdata", "cnb", "buildpacks"),
			filepath.Join("testdata", "cnb", "extensions"),
		)
		h.AssertNil(t, err)
	})

	when(".Lookup", func() {
		when("kind is buildpack", func() {
			it("returns descriptor from buildpack.toml", func() {
				bp, err := dirStore.Lookup(buildpack.KindBuildpack, "A", "v1")
				h.AssertNil(t, err)

				h.AssertEq(t, bp.API(), "0.7")
				h.AssertEq(t, bp.Homepage(), "Buildpack A Homepage")
			})
		})

		when("kind is extension", func() {
			it("returns descriptor from extension.toml", func() {
				ext, err := dirStore.Lookup(buildpack.KindExtension, "A", "v1")
				h.AssertNil(t, err)

				h.AssertEq(t, ext.API(), "0.9")
				h.AssertEq(t, ext.Homepage(), "Extension A Homepage")
			})
		})
	})

	when(".LookupBp", func() {
		it("returns buildpack from buildpack.toml", func() {
			bp, err := dirStore.LookupBp("A", "v1")
			h.AssertNil(t, err)

			h.AssertEq(t, bp.Buildpack.ID, "A")
			h.AssertEq(t, bp.Buildpack.Version, "v1")
		})
	})

	when(".LookupExt", func() {
		it("returns extension from extension.toml", func() {
			ext, err := dirStore.LookupExt("A", "v1")
			h.AssertNil(t, err)

			h.AssertEq(t, ext.Extension.ID, "A")
			h.AssertEq(t, ext.Extension.Version, "v1")
		})
	})
}
