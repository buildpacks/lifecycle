//go:build linux
// +build linux

package fsutil_test

import (
	"github.com/buildpacks/lifecycle/internal/fsutil"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestDetectorUnix(t *testing.T) {
	spec.Run(t, "DetectorUnix", testDetectorUnix, spec.Report(report.Terminal{}))
}

func testDetectorUnix(t *testing.T, when spec.G, it spec.S) {
	when("we should have a file", func() {
		it("returns true correctly", func() {
			h.AssertEq(t, (&fsutil.Detect{}).HasLinuxFile(), true)
		})
		it("returns the file contents", func() {
			s, err := (&fsutil.Detect{}).ReadLinuxFile()
			h.AssertNil(t, err)
			h.AssertStringContains(t, s, "NAME")
		})

	})
}
