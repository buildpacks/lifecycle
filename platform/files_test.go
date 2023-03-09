package platform_test

import (
	"os"
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
				stackMD = &platform.StackMetadata{RunImage: platform.RunImageForExport{
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
					PreviousImage: &platform.ImageIdentifier{Reference: "previous-img"},
					Metadata: platform.LayersMetadata{
						Stack: platform.StackMetadata{
							RunImage: platform.RunImageForExport{Image: "imagine that"},
						},
					},
					RunImage: &platform.RunImage{Reference: "some-ref"},
				}
				f := h.TempFile(t, "", "")
				h.AssertNil(t, amd.WriteTOML(f))
				amd2, err := platform.ReadAnalyzed(f, nil)
				h.AssertNil(t, err)
				h.AssertEq(t, amd.PreviousImageRef(), amd2.PreviousImageRef())
				h.AssertEq(t, amd.Metadata, amd2.Metadata)
				h.AssertEq(t, amd.BuildImage, amd2.BuildImage)
			})
			it("serializes to the old format", func() {
				amd := platform.AnalyzedMetadata{
					PreviousImage: &platform.ImageIdentifier{Reference: "previous-img"},
					Metadata: platform.LayersMetadata{
						Stack: platform.StackMetadata{
							RunImage: platform.RunImageForExport{Image: "imagine that"},
						},
					},
					RunImage: &platform.RunImage{Reference: "some-ref"},
				}
				f := h.TempFile(t, "", "")
				h.AssertNil(t, amd.WriteTOML(f))
				contents, err := os.ReadFile(f)
				h.AssertNil(t, err)
				expectedContents := `[image]
  reference = "previous-img"

[metadata]
  [metadata.config]
    sha = ""
  [metadata.launcher]
    sha = ""
  [metadata.process-types]
    sha = ""
  [metadata.run-image]
    top-layer = ""
    reference = ""
  [metadata.stack]
    [metadata.stack.run-image]
      image = "imagine that"

[run-image]
  reference = "some-ref"
`
				h.AssertEq(t, string(contents), expectedContents)
			})
		})
		when("it is new it stays new", func() {
			it("serializes and deserializes", func() {
				amd := platform.AnalyzedMetadata{
					PreviousImage: &platform.ImageIdentifier{Reference: "the image formerly known as prince"},
					RunImage: &platform.RunImage{
						Reference: "librarian",
						Target:    &platform.TargetMetadata{TargetPartial: buildpack.TargetPartial{OS: "os/2 warp", Arch: "486"}},
					},
					BuildImage: &platform.ImageIdentifier{Reference: "implementation"},
				}
				f := h.TempFile(t, "", "")
				h.AssertNil(t, amd.WriteTOML(f))
				amd2, err := platform.ReadAnalyzed(f, nil)
				h.AssertNil(t, err)
				h.AssertEq(t, amd.PreviousImageRef(), amd2.PreviousImageRef())
				h.AssertEq(t, amd.Metadata, amd2.Metadata)
				h.AssertEq(t, amd.BuildImage, amd2.BuildImage)
			})
		})
		when("TargetMetadata#IsSatisfiedBy", func() {
			it("requires equality of OS and Arch", func() {
				d := platform.TargetMetadata{TargetPartial: buildpack.TargetPartial{OS: "Win95", Arch: "Pentium"}}

				if d.IsSatisfiedBy(&buildpack.TargetMetadata{TargetPartial: buildpack.TargetPartial{OS: "Win98", Arch: d.Arch}}) {
					t.Fatal("TargetMetadata with different OS were equal")
				}
				if d.IsSatisfiedBy(&buildpack.TargetMetadata{TargetPartial: buildpack.TargetPartial{OS: d.OS, Arch: "Pentium MMX"}}) {
					t.Fatal("TargetMetadata with different Arch were equal")
				}
				if !d.IsSatisfiedBy(&buildpack.TargetMetadata{TargetPartial: buildpack.TargetPartial{OS: d.OS, Arch: d.Arch, ArchVariant: "MMX"}}) {
					t.Fatal("blank arch variant was not treated as wildcard")
				}
				if !d.IsSatisfiedBy(&buildpack.TargetMetadata{
					TargetPartial: buildpack.TargetPartial{OS: d.OS, Arch: d.Arch},
					Distributions: []buildpack.DistributionMetadata{{Name: "a", Version: "2"}},
				}) {
					t.Fatal("blank distributions list was not treated as wildcard")
				}

				d.Distribution = &buildpack.DistributionMetadata{Name: "A", Version: "1"}
				if d.IsSatisfiedBy(&buildpack.TargetMetadata{TargetPartial: buildpack.TargetPartial{OS: d.OS, Arch: d.Arch}, Distributions: []buildpack.DistributionMetadata{{Name: "g", Version: "2"}, {Name: "B", Version: "2"}}}) {
					t.Fatal("unsatisfactory distribution lists were treated as satisfying")
				}
				if !d.IsSatisfiedBy(&buildpack.TargetMetadata{TargetPartial: buildpack.TargetPartial{OS: d.OS, Arch: d.Arch}, Distributions: []buildpack.DistributionMetadata{}}) {
					t.Fatal("blank distributions list was not treated as wildcard")
				}
				if !d.IsSatisfiedBy(&buildpack.TargetMetadata{TargetPartial: buildpack.TargetPartial{OS: d.OS, Arch: d.Arch}, Distributions: []buildpack.DistributionMetadata{{Name: "B", Version: "2"}, {Name: "A", Version: "1"}}}) {
					t.Fatal("distributions list including target's distribution not recognized as satisfying")
				}
			})
		})
	})
}
