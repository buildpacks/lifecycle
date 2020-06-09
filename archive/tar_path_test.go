package archive_test

import (
	"runtime"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/archive"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestTarPath(t *testing.T) {
	spec.Run(t, "testTarPath", testTarPath, spec.Report(report.Terminal{}))
}

type testCase struct {
	path     string
	expected string
}

func testTarPath(t *testing.T, when spec.G, it spec.S) {
	when("#TarPath", func() {
		testCases := []testCase{
			{`/`, `/`},
			{`/foo/bar/`, `/foo/bar/`},
			{`foo/bar/`, `foo/bar/`},
		}

		if runtime.GOOS == "windows" {
			testCases = []testCase{
				{`c:\`, `/`},
				{`C:\`, `/`},
				{`d:\`, `/`},
				{`c:\foo\`, `/foo/`},
				{`c:\foo\bar\`, `/foo/bar/`},
				{`c:\foo\bar\baz.txt`, `/foo/bar/baz.txt`},
				{`foo`, `foo`},
			}
		}

		for _, tc := range testCases {
			path := tc.path
			expected := tc.expected
			it("removes volume and converts slashes", func() {
				actual := archive.TarPath(path)
				h.AssertEq(t, actual, expected)
			})
		}
	})
}
