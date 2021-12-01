package io_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/internal/io"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestIO(t *testing.T) {
	spec.Run(t, "IO", testIO, spec.Report(report.Terminal{}))
}

func testIO(t *testing.T, when spec.G, it spec.S) {
	when(".Copy", func() {
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

		it("should copy files", func() {
			var (
				src  = filepath.Join(tmpDir, "src.txt")
				dest = filepath.Join(tmpDir, "dest.txt")
			)

			h.Mkfile(t, "some-file-content", src)

			err := io.Copy(src, dest)
			if err != nil {
				t.Fatal(err)
			}

			result := h.MustReadFile(t, dest)
			h.AssertEq(t, string(result), "some-file-content")
		})
	})
}
