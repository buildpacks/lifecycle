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

	"github.com/sclevine/lifecycle"
)

func TestDetector(t *testing.T) {
	spec.Run(t, "Detector", testDetector, spec.Report(report.Terminal{}))
}

//go:generate mockgen -package mocks -destination testmock/env.go github.com/sclevine/lifecycle Env

func testDetector(t *testing.T, when spec.G, it spec.S) {
	var (
		list   lifecycle.BuildpackList
		tmpDir string
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		buildpackDir := filepath.Join("testdata", "buildpack")
		list = lifecycle.BuildpackList{
			{
				{Name: "buildpack1-name", Dir: buildpackDir},
				{Name: "buildpack2-name", Dir: buildpackDir},
				{Name: "buildpack3-name", Dir: buildpackDir},
				{Name: "buildpack4-name", Dir: buildpackDir},
			},
			{
				{Name: "buildpack1-name", Dir: buildpackDir},
				{Name: "buildpack2-name", Dir: buildpackDir},
				{Name: "buildpack3-name", Dir: buildpackDir},
			},
			{
				{Name: "buildpack1-name", Dir: buildpackDir},
				{Name: "buildpack2-name", Dir: buildpackDir},
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
			if group := list.Detect(tmpDir, l); !reflect.DeepEqual(group, list[1]) {
				t.Fatalf("Unexpected group: %#v\n", group)
			}
			if strings.HasSuffix(out.String(),
				"3\nGroup: buildpack1-name: pass | buildpack2-name: pass | buildpack3-name: pass",
			) {
				t.Fatalf("Unexpected log: %s\n", out)
			}
		})

		it("should return empty if no groups match", func() {
			mkfile(t, "1", filepath.Join(tmpDir, "add"))
			mkfile(t, "1", filepath.Join(tmpDir, "last"))
			out := &bytes.Buffer{}
			l := log.New(io.MultiWriter(out, it.Out()), "", 0)
			if group := list.Detect(tmpDir, l); len(group) > 0 {
				t.Fatalf("Unexpected group: %#v\n", group)
			}
			if strings.HasSuffix(out.String(),
				"2\nGroup: buildpack1-name: pass | buildpack2-name: fail",
			) {
				t.Fatalf("Unexpected log: %s\n", out)
			}
		})

		it("should return empty there is an error", func() {
			out := &bytes.Buffer{}
			l := log.New(io.MultiWriter(out, it.Out()), "", 0)
			if group := list.Detect(tmpDir, l); len(group) > 0 {
				t.Fatalf("Unexpected group: %#v\n", group)
			}
			if strings.HasSuffix(out.String(),
				"No such file or directory\nGroup: buildpack1-name: error (1) | buildpack2-name: error (1)",
			) {
				t.Fatalf("Unexpected log: %s\n", out)
			}
		})
	})
}
