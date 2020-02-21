package env

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
)

func TestLaunchEnv(t *testing.T) {
	spec.Run(t, "LaunchEnv", testLaunchEnv, spec.Report(report.Terminal{}))
}

func testLaunchEnv(t *testing.T, when spec.G, it spec.S) {
	when("#NewLaunchEnv", func() {
		it("blacklists vars", func() {
			env := NewLaunchEnv([]string{
				"CNB_APP_DIR=blacklisted",
				"CNB_LAYERS_DIR=blacklisted",
				"CNB_PROCESS_TYPE=blacklisted",
				"CNB_FOO=not-blacklisted",
			})
			if s := cmp.Diff(env.List(), []string{
				"CNB_FOO=not-blacklisted",
			}); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})

		it("allows keys with '='", func() {
			env := NewLaunchEnv([]string{
				"CNB_FOO=some=key",
			})
			if s := cmp.Diff(env.List(), []string{
				"CNB_FOO=some=key",
			}); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})

		it("assign the Launch time root dir map", func() {
			env := NewLaunchEnv([]string{})
			if s := cmp.Diff(env.RootDirMap, POSIXLaunchEnv); s != "" {
				t.Fatalf("Unexpected root dir map\n%s\n", s)
			}
		})
	})
}
