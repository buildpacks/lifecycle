package lifecycle_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/lifecycle"
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
}
