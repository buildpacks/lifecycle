//go:build linux

package fsutil_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/internal/fsutil"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDetectorUnix(t *testing.T) {
	t.Run("we should have a file", func(t *testing.T) {
		t.Run("returns true correctly", func(t *testing.T) {
			h.AssertEq(t, (&fsutil.DefaultDetector{}).HasSystemdFile(), true)
		})
		t.Run("returns the file contents", func(t *testing.T) {
			s, err := (&fsutil.DefaultDetector{}).ReadSystemdFile()
			h.AssertNil(t, err)
			h.AssertStringContains(t, s, "NAME")
		})
	})
}
