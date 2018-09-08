package lifecycle_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
)

func TestMap(t *testing.T) {
	spec.Run(t, "Map", testMap, spec.Report(report.Terminal{}))
}

func testMap(t *testing.T, when spec.G, it spec.S) {
	when(".NewBuildpackMap", func() {
		it("should return a map of buildpacks in the provided directory", func() {
			tmpDir, err := ioutil.TempDir("", "lifecycle.test")
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			mkdir(t,
				filepath.Join(tmpDir, "buildpack1", "version1"),
				filepath.Join(tmpDir, "buildpack2", "version2.1"),
				filepath.Join(tmpDir, "buildpack2", "version2.2"),
				filepath.Join(tmpDir, "buildpack2", "version2.3"),
				filepath.Join(tmpDir, "buildpack2", "version2.4"),
				filepath.Join(tmpDir, "buildpack3", "version3"),
				filepath.Join(tmpDir, "buildpack4", "version4"),
			)
			mkBuildpackTOML(t, tmpDir, "buildpack1", "version1")
			mkBuildpackTOML(t, tmpDir, "buildpack2", "version2.1")
			mkBuildpackTOML(t, tmpDir, "buildpack2", "version2.2")
			mkfile(t, "other",
				filepath.Join(tmpDir, "buildpack2", "version2.3", "not-buildpack.toml"),
				filepath.Join(tmpDir, "buildpack3", "version3", "not-buildpack.toml"),
			)
			m, err := lifecycle.NewBuildpackMap(tmpDir)
			if !reflect.DeepEqual(m, lifecycle.BuildpackMap{
				"buildpack1@version1": {
					ID:      "buildpack1",
					Name:    "buildpack1-name",
					Version: "version1",
					Dir:     filepath.Join(tmpDir, "buildpack1", "version1"),
				},
				"buildpack2@version2.1": {
					ID:      "buildpack2",
					Name:    "buildpack2-name",
					Version: "version2.1",
					Dir:     filepath.Join(tmpDir, "buildpack2", "version2.1"),
				},
				"buildpack2@version2.2": {
					ID:      "buildpack2",
					Name:    "buildpack2-name",
					Version: "version2.2",
					Dir:     filepath.Join(tmpDir, "buildpack2", "version2.2"),
				},
			}) {
				t.Fatalf("Unexpected map: %#v\n", m)
			}
		})
	})

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

		it("should return an ordering of buildpacks", func() {
			m := lifecycle.BuildpackMap{
				"buildpack1@version1.1": {Name: "buildpack1-1.1"},
				"buildpack1@version1.2": {Name: "buildpack1-1.2"},
				"buildpack2@latest":     {Name: "buildpack2"},
			}
			mkfile(t, `groups = [{ buildpacks = [{id = "buildpack1", version = "version1.1"}, {id = "buildpack2", optional = true}] }]`,
				filepath.Join(tmpDir, "order.toml"),
			)
			actual, err := m.ReadOrder(filepath.Join(tmpDir, "order.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, lifecycle.BuildpackOrder{
				{Buildpacks: []*lifecycle.Buildpack{{Name: "buildpack1-1.1"}, {Name: "buildpack2", Optional: true}}},
			}) {
				t.Fatalf("Unexpected list: %#v\n", actual)
			}
		})
	})

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

		it("should return a group of buildpacks", func() {
			m := lifecycle.BuildpackMap{
				"buildpack1@version1.1": {Name: "buildpack1-1.1"},
				"buildpack1@version1.2": {Name: "buildpack1-1.2"},
				"buildpack2@latest":     {Name: "buildpack2"},
			}
			mkfile(t, `buildpacks = [{id = "buildpack1", version = "version1.1"}, {id = "buildpack2", optional = true}]`,
				filepath.Join(tmpDir, "group.toml"),
			)
			actual, err := m.ReadGroup(filepath.Join(tmpDir, "group.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actual, &lifecycle.BuildpackGroup{
				Buildpacks: []*lifecycle.Buildpack{{Name: "buildpack1-1.1"}, {Name: "buildpack2", Optional: true}},
			}) {
				t.Fatalf("Unexpected list: %#v\n", actual)
			}
		})
	})

	when("#Write", func() {
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

		it("should write only ID and version", func() {
			group := lifecycle.BuildpackGroup{
				Buildpacks: []*lifecycle.Buildpack{{ID: "a", Name: "b", Version: "v", Dir: "d"}},
			}
			if err := group.Write(filepath.Join(tmpDir, "group.toml")); err != nil {
				t.Fatal(err)
			}
			b, err := ioutil.ReadFile(filepath.Join(tmpDir, "group.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(string(b), "[[buildpacks]]\n  id = \"a\"\n  version = \"v\"\n"); diff != "" {
				t.Fatalf(`toml did not match: (-got +want)\n%s`, diff)
			}
		})
	})
}

const buildpackTOML = `
[buildpack]
id = "%[1]s"
name = "%[1]s-name"
version = "%[2]s"
dir = "none"
`

func mkBuildpackTOML(t *testing.T, dir, id, version string) {
	mkfile(t, fmt.Sprintf(buildpackTOML, id, version),
		filepath.Join(dir, id, version, "buildpack.toml"),
	)
}
