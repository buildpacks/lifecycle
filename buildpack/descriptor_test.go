package buildpack_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestUtils(t *testing.T) {
	spec.Run(t, "Descriptor", testDescriptor, spec.Report(report.Terminal{}))
}

func testDescriptor(t *testing.T, when spec.G, it spec.S) {
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
			actual, _, err := buildpack.ReadOrder(filepath.Join(tmpDir, "order.toml"))
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

		when("there are extensions", func() {
			it("should return an ordering of buildpacks and an ordering of extensions", func() {
				h.Mkfile(t,
					"[[order]]\n"+
						`group = [{id = "A", version = "v1"}, {id = "B", optional = true}]`+"\n"+
						"[[order]]\n"+
						`group = [{id = "C"}, {}]`+"\n"+
						"[[order-ext]]\n"+
						`group = [{id = "D"}, {}]`+"\n",
					filepath.Join(tmpDir, "order.toml"),
				)
				foundOrder, foundOrderExt, err := buildpack.ReadOrder(filepath.Join(tmpDir, "order.toml"))
				if err != nil {
					t.Fatalf("Unexpected error:\n%s\n", err)
				}
				if s := cmp.Diff(foundOrder, buildpack.Order{
					{Group: []buildpack.GroupBuildpack{{ID: "A", Version: "v1"}, {ID: "B", Optional: true}}},
					{Group: []buildpack.GroupBuildpack{{ID: "C"}, {}}},
				}); s != "" {
					t.Fatalf("Unexpected list:\n%s\n", s)
				}
				if s := cmp.Diff(foundOrderExt, buildpack.Order{
					{Group: []buildpack.GroupBuildpack{{ID: "D"}, {}}},
				}); s != "" {
					t.Fatalf("Unexpected list:\n%s\n", s)
				}
			})
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
			actual, err := buildpack.ReadGroup(filepath.Join(tmpDir, "group.toml"))
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
}
