//go:build linux
// +build linux

package fsutil_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/internal/fsutil"
	h "github.com/buildpacks/lifecycle/testhelpers"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestDetectorUnix(t *testing.T) {
	spec.Run(t, "DetectorUnix", testDetectorUnix, spec.Report(report.Terminal{}))
}

func testDetectorUnix(t *testing.T, when spec.G, it spec.S) {
	when("we should have a file", func() {
		it("returns true correctly", func() {
			h.AssertEq(t, (&fsutil.DefaultDetector{}).HasSystemdFile(), true)
		})
		it("returns the file contents", func() {
			s, err := (&fsutil.DefaultDetector{}).ReadSystemdFile()
			h.AssertNil(t, err)
			h.AssertStringContains(t, s, "NAME")
		})
	})
}
