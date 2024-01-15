//go:build windows || darwin || freebsd
// +build windows darwin freebsd

package fsutil_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/internal/fsutil"
	h "github.com/buildpacks/lifecycle/testhelpers"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestDetectorNonUnix(t *testing.T) {
	spec.Run(t, "DetectorNonUnix", testDetectorNonUnix, spec.Report(report.Terminal{}))
}

func testDetectorNonUnix(t *testing.T, when spec.G, it spec.S) {
	when("we don't have a file", func() {
		it("returns false correctly", func() {
			h.AssertEq(t, (&fsutil.Detect{}).HasSystemdFile(), false)
		})
		it("returns an error correctly", func() {
			_, err := (&fsutil.Detect{}).ReadSystemdFile()
			h.AssertNotNil(t, err)
		})
	})
}
