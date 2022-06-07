package platform_test

import (
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
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

	when(".Lookup", func() {
		when("kind is buildpack", func() {
			it("returns buildpack from buildpack.toml", func() {
				bp, err := dirStore.Lookup(buildpack.KindBuildpack, "A", "v1")
				h.AssertNil(t, err)

				config := bp.ConfigFile()
				h.AssertEq(t, config.Buildpack.ID, "A")
				h.AssertEq(t, config.Buildpack.Version, "v1")
			})
		})

		when("kind is extension", func() {
			it("returns extension from extension.toml", func() {
				ext, err := dirStore.Lookup(buildpack.KindExtension, "A", "v1")
				h.AssertNil(t, err)

				config := ext.ConfigFile()
				h.AssertEq(t, config.Extension.ID, "A")
				h.AssertEq(t, config.Extension.Version, "v1")
			})
		})
	})
}
