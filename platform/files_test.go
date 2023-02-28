package platform_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestFiles(t *testing.T) {
	spec.Run(t, "Files", testFiles)
}

func testFiles(t *testing.T, when spec.G, it spec.S) {
	when("metadata.toml", func() {
		when("MarshalJSON", func() {
			var (
				buildMD    *platform.BuildMetadata
				buildpacks []buildpack.GroupElement
			)

			it.Before(func() {
				buildpacks = []buildpack.GroupElement{
					{ID: "A", Version: "v1"},
				}
				buildMD = &platform.BuildMetadata{
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

	when("stack.toml", func() {
		when("BestRunImageMirror", func() {
			var stackMD *platform.StackMetadata

			it.Before(func() {
				stackMD = &platform.StackMetadata{RunImage: platform.RunImageMetadata{
					Image: "first.com/org/repo",
					Mirrors: []string{
						"myorg/myrepo",
						"zonal.gcr.io/org/repo",
						"gcr.io/org/repo",
					},
				}}
			})

			when("repoName is dockerhub", func() {
				it("returns the dockerhub image", func() {
					name, err := stackMD.BestRunImageMirror("index.docker.io")
					h.AssertNil(t, err)
					h.AssertEq(t, name, "myorg/myrepo")
				})
			})

			when("registry is gcr.io", func() {
				it("returns the gcr.io image", func() {
					name, err := stackMD.BestRunImageMirror("gcr.io")
					h.AssertNil(t, err)
					h.AssertEq(t, name, "gcr.io/org/repo")
				})

				when("registry is zonal.gcr.io", func() {
					it("returns the gcr image", func() {
						name, err := stackMD.BestRunImageMirror("zonal.gcr.io")
						h.AssertNil(t, err)
						h.AssertEq(t, name, "zonal.gcr.io/org/repo")
					})
				})

				when("registry is missingzone.gcr.io", func() {
					it("returns the run image", func() {
						name, err := stackMD.BestRunImageMirror("missingzone.gcr.io")
						h.AssertNil(t, err)
						h.AssertEq(t, name, "first.com/org/repo")
					})
				})
			})

			when("one of the images is non-parsable", func() {
				it.Before(func() {
					stackMD.RunImage.Mirrors = []string{"as@ohd@as@op", "gcr.io/myorg/myrepo"}
				})

				it("skips over it", func() {
					name, err := stackMD.BestRunImageMirror("gcr.io")
					h.AssertNil(t, err)
					h.AssertEq(t, name, "gcr.io/myorg/myrepo")
				})
			})
		})
	})
	when("analyzed.toml", func() {
		when("it is old it stays old", func() {
			it("serializes and deserializes", func() {
				amd := platform.AnalyzedMetadata{
					PreviousImageRef: "previous-img",
					Metadata: platform.LayersMetadata{
						Stack: platform.StackMetadata{
							RunImage: platform.RunImageMetadata{Image: "imagine that"},
						},
					},
					RunImage: platform.RunImage{Reference: "some-ref"},
				}
				f := h.TempFile(t, "", "")
				h.AssertNil(t, amd.WriteTOML(f))
				amd2, err := platform.ReadAnalyzed(f, nil)
				h.AssertNil(t, err)
				h.AssertEq(t, amd.PreviousImageRef, amd2.PreviousImageRef)
				h.AssertEq(t, amd.Metadata, amd2.Metadata)
				h.AssertEq(t, amd.API, amd2.API)
				h.AssertEq(t, amd.BuildImage, amd2.BuildImage)
			})
		})
		when("it is new it stays new", func() {
			it("serializes and deserializes", func() {
				amd := platform.AnalyzedMetadata{
					PreviousImageRef: "the image formerly known as prince",
					RunImage: platform.RunImage{
						Reference: "librarian",
						Target: buildpack.TargetMetadata{
							Os:   "os/2 warp",
							Arch: "486",
						},
					},
					BuildImage: &platform.ImageIdentifier{Reference: "implementation"},
					API:        "0.12",
				}
				f := h.TempFile(t, "", "")
				h.AssertNil(t, amd.WriteTOML(f))
				amd2, err := platform.ReadAnalyzed(f, nil)
				h.AssertNil(t, err)
				h.AssertEq(t, amd.PreviousImageRef, amd2.PreviousImageRef)
				h.AssertEq(t, amd.Metadata, amd2.Metadata)
				h.AssertEq(t, amd.API, amd2.API)
				h.AssertEq(t, amd.BuildImage, amd2.BuildImage)
			})
		})
	})
}
