//go:build darwin

package fsutil_test

import (
	"testing"

	"github.com/buildpacks/lifecycle/internal/fsutil"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDetectorNonUnix(t *testing.T) {
	t.Run("we don't have a file", func(t *testing.T) {
		t.Run("returns false correctly", func(t *testing.T) {
			h.AssertEq(t, (&fsutil.DefaultDetector{}).HasSystemdFile(), false)
		})
		t.Run("returns an error correctly", func(t *testing.T) {
			_, err := (&fsutil.DefaultDetector{}).ReadSystemdFile()
			h.AssertNotNil(t, err)
		})
	})
}
