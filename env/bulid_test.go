package env

import (
	"sort"
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
				"CNB_STACK_ID=some-stack-id",
				"NOT_WHITELIST=not-whitelisted",
				"PATH=some-path",
				"LD_LIBRARY_PATH=some-ld-library-path",
				"LIBRARY_PATH=some-library-path",
				"CPATH=some-cpath",
				"PKG_CONFIG_PATH=some-pkg-config-path",
			})
			out := env.List()
			sort.Strings(out)
			if s := cmp.Diff(out, []string{
				"CNB_STACK_ID=some-stack-id",
				"CPATH=some-cpath",
				"LD_LIBRARY_PATH=some-ld-library-path",
				"LIBRARY_PATH=some-library-path",
				"PATH=some-path",
				"PKG_CONFIG_PATH=some-pkg-config-path",
			}); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})

		it("allows keys with '='", func() {
			env := NewBuildEnv([]string{
				"CNB_STACK_ID=whitelist=true",
			})
			if s := cmp.Diff(env.List(), []string{
				"CNB_STACK_ID=whitelist=true",
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
