package lifecycle_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/platform/common"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestUtils(t *testing.T) {
	spec.Run(t, "Utils", testUtils, spec.Report(report.Terminal{}))
}

func testUtils(t *testing.T, when spec.G, it spec.S) {
	when(".ReadOrder", func() {
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

		it("should return an ordering of buildpacks", func() {
			h.Mkfile(t,
				"[[order]]\n"+
					`group = [{id = "A", version = "v1"}, {id = "B", optional = true}]`+"\n"+
					"[[order]]\n"+
					`group = [{id = "C"}, {}]`+"\n",
				filepath.Join(tmpDir, "order.toml"),
			)
			actual, err := lifecycle.ReadOrder(filepath.Join(tmpDir, "order.toml"))
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}
			if s := cmp.Diff(actual, buildpack.Order{
				{Group: []buildpack.GroupBuildpack{{ID: "A", Version: "v1"}, {ID: "B", Optional: true}}},
				{Group: []buildpack.GroupBuildpack{{ID: "C"}, {}}},
			}); s != "" {
				t.Fatalf("Unexpected list:\n%s\n", s)
			}
		})
	})

	when(".ReadGroup", func() {
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

		it("should return a group of buildpacks", func() {
			h.Mkfile(t, `group = [{id = "A", version = "v1"}, {id = "B", optional = true}]`,
				filepath.Join(tmpDir, "group.toml"),
			)
			actual, err := lifecycle.ReadGroup(filepath.Join(tmpDir, "group.toml"))
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}
			if s := cmp.Diff(actual, buildpack.Group{
				Group: []buildpack.GroupBuildpack{
					{ID: "A", Version: "v1"},
					{ID: "B", Optional: true},
				},
			}); s != "" {
				t.Fatalf("Unexpected list:\n%s\n", s)
			}
		})
	})

	when(".WriteTOML", func() {
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

		it("should write TOML", func() {
			group := buildpack.Group{Group: []buildpack.GroupBuildpack{{ID: "A", Version: "v1"}}}
			if err := lifecycle.WriteTOML(filepath.Join(tmpDir, "subdir", "group.toml"), group); err != nil {
				t.Fatal(err)
			}
			b := h.Rdfile(t, filepath.Join(tmpDir, "subdir", "group.toml"))
			if s := cmp.Diff(b,
				"[[group]]\n"+
					`  id = "A"`+"\n"+
					`  version = "v1"`+"\n",
			); s != "" {
				t.Fatalf("Unexpected TOML:\n%s\n", s)
			}
		})
	})

	when(".TruncateSha", func() {
		it("should truncate the sha", func() {
			actual := lifecycle.TruncateSha("ed649d0a36b218c476b64d61f85027477ef5742045799f45c8c353562279065a")
			if s := cmp.Diff(actual, "ed649d0a36b2"); s != "" {
				t.Fatalf("Unexpected sha:\n%s\n", s)
			}
		})

		it("should not truncate the sha with it's short", func() {
			sha := "not-a-sha"
			actual := lifecycle.TruncateSha(sha)
			if s := cmp.Diff(actual, sha); s != "" {
				t.Fatalf("Unexpected sha:\n%s\n", s)
			}
		})

		it("should remove the prefix", func() {
			sha := "sha256:ed649d0a36b218c476b64d61f85027477ef5742045799f45c8c353562279065a"
			actual := lifecycle.TruncateSha(sha)
			if s := cmp.Diff(actual, "ed649d0a36b2"); s != "" {
				t.Fatalf("Unexpected sha:\n%s\n", s)
			}
		})
	})

	when("ResolveRunImage", func() {
		when("there are no mirrors", func() {
			it("should return run-image", func() {
				md := common.StackMetadata{RunImage: common.StackRunImageMetadata{Image: "company/run:focal"}}
				dstImage := "someregistry/whatever"
				res, err := lifecycle.ResolveRunImage(md, dstImage)
				h.AssertNil(t, err)
				h.AssertEq(t, res, md.RunImage.Image)
			})
		})

		when("there are mirrors", func() {
			it("should return a run-image from the mirror matching the destination image", func() {
				md := common.StackMetadata{RunImage: common.StackRunImageMetadata{
					Image:   "company/run:focal",
					Mirrors: []string{"some.registry/_/run:focal"},
				}}
				dstImage := "some.registry/app/web"
				res, err := lifecycle.ResolveRunImage(md, dstImage)
				h.AssertNil(t, err)
				h.AssertEq(t, res, "some.registry/_/run:focal")
			})
		})

		when("there is no run image defined", func() {
			it("should fail", func() {
				md := common.StackMetadata{}
				dstImage := "someregistry/whatever"
				_, err := lifecycle.ResolveRunImage(md, dstImage)
				h.AssertError(t, err, "a run image must be specified when there is no stack metadata available")
			})
		})
	})
}
