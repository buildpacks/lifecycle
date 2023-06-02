package files_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestBuildMetadata(t *testing.T) {
	spec.Run(t, "BuildMetadata", testBuildMetadata)
}

func testBuildMetadata(t *testing.T, when spec.G, it spec.S) {
	when("metadata.toml", func() {
		when("MarshalJSON", func() {
			var (
				buildMD    *files.BuildMetadata
				buildpacks []buildpack.GroupElement
			)

			it.Before(func() {
				buildpacks = []buildpack.GroupElement{
					{ID: "A", Version: "v1"},
				}
				buildMD = &files.BuildMetadata{
					BOM: []buildpack.BOMEntry{{
						Require: buildpack.Require{
							Name: "some-dep",
						},
						Buildpack: buildpack.GroupElement{
							ID: "A", Version: "v1",
						},
					}},
					Buildpacks:  buildpacks,
					PlatformAPI: api.Platform.Latest(),
				}
			})

			it("omits bom", func() {
				b, err := buildMD.MarshalJSON()
				h.AssertNil(t, err)
				if s := cmp.Diff(string(b),
					`{"buildpacks":[{"id":"A","version":"v1"}],`+
						`"launcher":{"version":"","source":{"git":{"repository":"","commit":""}}},`+
						`"processes":null}`,
				); s != "" {
					t.Fatalf("Unexpected JSON:\n%s\n", s)
				}
			})

			when("Platform API < 0.9", func() {
				it.Before(func() {
					buildMD.PlatformAPI = api.MustParse("0.8")
				})

				it("does not omit bom", func() {
					b, err := buildMD.MarshalJSON()
					h.AssertNil(t, err)
					if s := cmp.Diff(string(b),
						`{"bom":[{"name":"some-dep","metadata":null,"buildpack":{"id":"A","version":"v1"}}],`+
							`"buildpacks":[{"id":"A","version":"v1"}],`+
							`"launcher":{"version":"","source":{"git":{"repository":"","commit":""}}},`+
							`"processes":null}`,
					); s != "" {
						t.Fatalf("Unexpected JSON:\n%s\n", s)
					}
				})
			})

			when("missing platform", func() {
				it.Before(func() {
					buildMD.PlatformAPI = nil
				})

				it("does not omit bom", func() {
					b, err := buildMD.MarshalJSON()
					h.AssertNil(t, err)
					if s := cmp.Diff(string(b),
						`{"bom":[{"name":"some-dep","metadata":null,"buildpack":{"id":"A","version":"v1"}}],`+
							`"buildpacks":[{"id":"A","version":"v1"}],`+
							`"launcher":{"version":"","source":{"git":{"repository":"","commit":""}}},`+
							`"processes":null}`,
					); s != "" {
						t.Fatalf("Unexpected JSON:\n%s\n", s)
					}
				})
			})
		})
	})
}
