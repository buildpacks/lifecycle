package encoding_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/encoding"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestEncoding(t *testing.T) {
	spec.Run(t, "Utils", testEncoding, spec.Report(report.Terminal{}))
}

func testEncoding(t *testing.T, when spec.G, it spec.S) {
	when(".WriteJSON", func() {
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

		it("should write JSON", func() {
			group := buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}}
			if err := encoding.WriteJSON(filepath.Join(tmpDir, "subdir", "group.toml"), group); err != nil {
				t.Fatal(err)
			}
			b := h.Rdfile(t, filepath.Join(tmpDir, "subdir", "group.toml"))
			if s := cmp.Diff(b,
				"{\"Group\":[{\"id\":\"A\",\"version\":\"v1\"}]}\n",
			); s != "" {
				t.Fatalf("Unexpected JSON:\n%s\n", s)
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
			group := buildpack.Group{Group: []buildpack.GroupElement{{ID: "A", Version: "v1"}}}
			if err := encoding.WriteTOML(filepath.Join(tmpDir, "subdir", "group.toml"), group); err != nil {
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
}
