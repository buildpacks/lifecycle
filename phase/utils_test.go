package phase_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/buildpacks/lifecycle/phase"
)

func TestUtils(t *testing.T) {
	t.Run(".TruncateSha", func(t *testing.T) {
		t.Run("should truncate the sha", func(t *testing.T) {
			actual := phase.TruncateSha("ed649d0a36b218c476b64d61f85027477ef5742045799f45c8c353562279065a")
			if s := cmp.Diff(actual, "ed649d0a36b2"); s != "" {
				t.Fatalf("Unexpected sha:\n%s\n", s)
			}
		})
		t.Run("should not truncate the sha with it's short", func(t *testing.T) {
			sha := "not-a-sha"
			actual := phase.TruncateSha(sha)
			if s := cmp.Diff(actual, sha); s != "" {
				t.Fatalf("Unexpected sha:\n%s\n", s)
			}
		})
		t.Run("should remove the prefix", func(t *testing.T) {
			sha := "sha256:ed649d0a36b218c476b64d61f85027477ef5742045799f45c8c353562279065a"
			actual := phase.TruncateSha(sha)
			if s := cmp.Diff(actual, "ed649d0a36b2"); s != "" {
				t.Fatalf("Unexpected sha:\n%s\n", s)
			}
		})
	})
}
