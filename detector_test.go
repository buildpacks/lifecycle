package lifecycle_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
)

func TestDetector(t *testing.T) {
	spec.Run(t, "Detector", testDetector, spec.Report(report.Terminal{}))
}

func testDetector(t *testing.T, when spec.G, it spec.S) {
	var (
		list   lifecycle.BuildpackOrder
		tmpDir string
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		buildpackDir := filepath.Join("testdata", "buildpack")
		list = lifecycle.BuildpackOrder{
			{
				Buildpacks: []*lifecycle.Buildpack{
					{Name: "buildpack1-name", Dir: buildpackDir},
					{Name: "buildpack2-name", Dir: buildpackDir},
					{Name: "buildpack3-name", Dir: buildpackDir},
					{Name: "buildpack4-name", Dir: buildpackDir},
				},
				Repository: "repository1",
			},
			{
				Buildpacks: []*lifecycle.Buildpack{
					{Name: "buildpack1-name", Dir: buildpackDir},
					{Name: "buildpack2-name", Dir: buildpackDir},
					{Name: "buildpack3-name", Dir: buildpackDir},
				},
				Repository: "repository2",
			},
			{
				Buildpacks: []*lifecycle.Buildpack{
					{Name: "buildpack1-name", Dir: buildpackDir},
					{Name: "buildpack2-name", Dir: buildpackDir},
				},
				Repository: "repository3",
			},
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#Detect", func() {
		it("should return the first matching group", func() {
			mkfile(t, "1", filepath.Join(tmpDir, "add"))
			mkfile(t, "3", filepath.Join(tmpDir, "last"))
			out := &bytes.Buffer{}
			l := log.New(io.MultiWriter(out, it.Out()), "", 0)

			if info, group := list.Detect(l, tmpDir); !reflect.DeepEqual(*group, list[1]) {
				t.Fatalf("Unexpected group: %#v\n", group)
			} else if s := string(info); s != "1 = true\n2 = true\n3 = true\n" {
				t.Fatalf("Unexpected info: %s\n", s)
			}

			if !strings.HasSuffix(out.String(),
				"3 = true\nGroup: buildpack1-name: pass | buildpack2-name: pass | buildpack3-name: pass\n",
			) {
				t.Fatalf("Unexpected log: %s\n", out)
			}
		})

		it("should return empty if no groups match", func() {
			mkfile(t, "1", filepath.Join(tmpDir, "add"))
			mkfile(t, "1", filepath.Join(tmpDir, "last"))
			out := &bytes.Buffer{}
			l := log.New(io.MultiWriter(out, it.Out()), "", 0)

			if info, group := list.Detect(l, tmpDir); group != nil {
				t.Fatalf("Unexpected group: %#v\n", group)
			} else if len(info) > 0 {
				t.Fatalf("Unexpected info: %s\n", string(info))
			}

			if !strings.HasSuffix(out.String(),
				"2 = true\nGroup: buildpack1-name: pass | buildpack2-name: fail\n",
			) {
				t.Fatalf("Unexpected log: %s\n", out)
			}
		})

		it("should return empty there is an error", func() {
			mkfile(t, "1", filepath.Join(tmpDir, "add"))
			mkfile(t, "error", filepath.Join(tmpDir, "error"))
			out := &bytes.Buffer{}
			l := log.New(io.MultiWriter(out, it.Out()), "", 0)

			if info, group := list.Detect(l, tmpDir); group != nil {
				t.Fatalf("Unexpected group: %#v\n", group)
			} else if len(info) > 0 {
				t.Fatalf("Unexpected info: %s\n", string(info))
			}

			if !strings.HasSuffix(out.String(),
				"2 = true\nGroup: buildpack1-name: error (1) | buildpack2-name: error (1)\n",
			) {
				t.Fatalf("Unexpected log: %s\n", out)
			}
		})
	})
}
