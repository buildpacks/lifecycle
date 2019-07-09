package lifecycle_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildpack/lifecycle"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
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
			mkfile(t,
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
			if s := cmp.Diff(actual, lifecycle.BuildpackOrder{
				{Group: []lifecycle.Buildpack{{ID: "A", Version: "v1"}, {ID: "B", Optional: true}}},
				{Group: []lifecycle.Buildpack{{ID: "C"}, {}}},
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
			mkfile(t, `group = [{id = "A", version = "v1"}, {id = "B", optional = true}]`,
				filepath.Join(tmpDir, "group.toml"),
			)
			actual, err := lifecycle.ReadGroup(filepath.Join(tmpDir, "group.toml"))
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}
			if s := cmp.Diff(actual, lifecycle.BuildpackGroup{
				Group: []lifecycle.Buildpack{
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
			group := lifecycle.BuildpackGroup{Group: []lifecycle.Buildpack{{ID: "A", Version: "v1"}}}
			if err := lifecycle.WriteTOML(filepath.Join(tmpDir, "subdir", "group.toml"), group); err != nil {
				t.Fatal(err)
			}
			b := rdfile(t, filepath.Join(tmpDir, "subdir", "group.toml"))
			if s := cmp.Diff(string(b),
				"[[group]]\n"+
					`  id = "A"`+"\n"+
					`  version = "v1"`+"\n",
			); s != "" {
				t.Fatalf("Unexpected TOML:\n%s\n", s)
			}
		})
	})
}
