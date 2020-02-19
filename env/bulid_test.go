package env

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestBuildEnv(t *testing.T) {
	spec.Run(t, "BuildEnv", testBuildEnv, spec.Report(report.Terminal{}))
}

func testBuildEnv(t *testing.T, when spec.G, it spec.S) {
	when("#NewBuildEnv", func() {
		it("whitelists vars", func() {
			env := NewBuildEnv([]string{
				"CNB_STACK_ID=whitelisted",
				"CNB_FOO=not-whitelisted",
			})
			if s := cmp.Diff(env.List(), []string{
				"CNB_STACK_ID=whitelisted",
			}); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})

		it("assign the build time root dir map", func() {
			env := NewBuildEnv([]string{})
			if s := cmp.Diff(env.RootDirMap, POSIXBuildEnv); s != "" {
				t.Fatalf("Unexpected root dir map\n%s\n", s)
			}
		})
	})
}
