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
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestUtils(t *testing.T) {
	spec.Run(t, "Utils", testUtils, spec.Report(report.Terminal{}))
}

func testUtils(t *testing.T, when spec.G, it spec.S) {
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

			err := lifecycle.Copy(src, dest)
			if err != nil {
				t.Fatal(err)
			}

			result := h.MustReadFile(t, dest)
			h.AssertEq(t, string(result), "some-file-content")
		})
	})
}
