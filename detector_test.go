package lifecycle_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"reflect"
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

	when.Pend("#Detect", func() {
		it("should return the first matching group", func() {
			// FIXME: data race on read-only app dir
			mkfile(t, "1", filepath.Join(tmpDir, "add"))
			mkfile(t, "3", filepath.Join(tmpDir, "last"))
			out := &bytes.Buffer{}
			l := log.New(io.MultiWriter(out, it.Out()), "", log.LstdFlags)
			if group := list.Detect(tmpDir, l); !reflect.DeepEqual(group, list[1]) {
				t.Fatalf("Unexpected group: %#v\n", group)
			}

			if result := read(t, filepath.Join(tmpDir, "result")); result != "3" {
				t.Fatalf("Unexpected result: %s\n", result)
			}
		})
	})
}
