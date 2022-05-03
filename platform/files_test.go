package platform_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
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
	when("group.toml", func() {
		when("#ReadGroup", func() {
			var tmpDir string

			it.Before(func() {
				var err error
				tmpDir, err = ioutil.TempDir("", "lifecycle.test")
				if err != nil {
					t.Fatal(err)
				}
			})

			it.After(func() {
				os.RemoveAll(tmpDir)
			})

			it("returns a group of buildpacks", func() {
				h.Mkfile(t, `group = [{id = "A", version = "v1"}, {id = "B", optional = true}]`,
					filepath.Join(tmpDir, "group.toml"),
				)
				actual, err := platform.ReadGroup(filepath.Join(tmpDir, "group.toml"))
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
				if s := cmp.Diff(actual, buildpack.Group{
					Group: []buildpack.GroupElement{
						{ID: "A", Version: "v1"},
						{ID: "B", Optional: true},
					},
				}); s != "" {
					t.Fatalf("Unexpected list:\n%s\n", s)
				}
			})
		})
	})

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

			when("platform api < 0.9", func() {
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

	when("order.toml", func() {
		when("#ReadOrder", func() {
			var tmpDir string

			it.Before(func() {
				var err error
				tmpDir, err = ioutil.TempDir("", "lifecycle.test")
				if err != nil {
					t.Fatal(err)
				}
			})

			it.After(func() {
				os.RemoveAll(tmpDir)
			})

			it("returns an ordering of buildpacks", func() {
				h.Mkfile(t,
					"[[order]]\n"+
						`group = [{id = "A", version = "v1"}, {id = "B", optional = true}]`+"\n"+
						"[[order]]\n"+
						`group = [{id = "C"}, {}]`+"\n",
					filepath.Join(tmpDir, "order.toml"),
				)
				actual, _, err := platform.ReadOrder(filepath.Join(tmpDir, "order.toml"))
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
				if s := cmp.Diff(actual, buildpack.Order{
					{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}, {ID: "B", Optional: true}}},
					{Group: []buildpack.GroupElement{{ID: "C"}, {}}},
				}); s != "" {
					t.Fatalf("Unexpected list:\n%s\n", s)
				}
			})

			when("there are extensions", func() {
				it("returns an ordering of buildpacks and an ordering of extensions", func() {
					h.Mkfile(t,
						"[[order]]\n"+
							`group = [{id = "A", version = "v1"}, {id = "B", optional = true}]`+"\n"+
							"[[order]]\n"+
							`group = [{id = "C"}, {}]`+"\n"+
							"[[order-ext]]\n"+
							`group = [{id = "D"}, {}]`+"\n",
						filepath.Join(tmpDir, "order.toml"),
					)
					foundOrder, foundOrderExt, err := platform.ReadOrder(filepath.Join(tmpDir, "order.toml"))
					if err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := cmp.Diff(foundOrder, buildpack.Order{
						{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}, {ID: "B", Optional: true}}},
						{Group: []buildpack.GroupElement{{ID: "C"}, {}}},
					}); s != "" {
						t.Fatalf("Unexpected list:\n%s\n", s)
					}
					if s := cmp.Diff(foundOrderExt, buildpack.Order{
						{Group: []buildpack.GroupElement{{ID: "D"}, {}}},
					}); s != "" {
						t.Fatalf("Unexpected list:\n%s\n", s)
					}
				})

				it("errors when order-ext contains a nested order", func() {
					// TODO
				})

				it("errors when order-ext contains buildpacks", func() {
					// TODO
				})

				it("errors when order contains extensions", func() {
					// TODO
				})
			})
		})
	})

	when("stack.toml", func() {
		when("BestRunImageMirror", func() {
			var stackMD *platform.StackMetadata

			it.Before(func() {
				stackMD = &platform.StackMetadata{RunImage: platform.StackRunImageMetadata{
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
}
