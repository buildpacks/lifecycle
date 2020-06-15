package env_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/env"
)

func TestLaunchEnv(t *testing.T) {
	spec.Run(t, "LaunchEnv", testLaunchEnv, spec.Report(report.Terminal{}))
}

func testLaunchEnv(t *testing.T, when spec.G, it spec.S) {
	when("#NewLaunchEnv", func() {
		it("blocklists vars", func() {
			lenv := env.NewLaunchEnv([]string{
				"CNB_APP_DIR=blocklisted",
				"CNB_LAYERS_DIR=blocklisted",
				"CNB_PROCESS_TYPE=blocklisted",
				"CNB_FOO=not-blocklisted",
			})
			if s := cmp.Diff(lenv.List(), []string{
				"CNB_FOO=not-blocklisted",
			}); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})

		it("allows keys with '='", func() {
			lenv := env.NewLaunchEnv([]string{
				"CNB_FOO=some=key",
			})
			if s := cmp.Diff(lenv.List(), []string{
				"CNB_FOO=some=key",
			}); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})

		it("assign the Launch time root dir map", func() {
			lenv := env.NewLaunchEnv([]string{})
			if s := cmp.Diff(lenv.RootDirMap, env.POSIXLaunchEnv); s != "" {
				t.Fatalf("Unexpected root dir map\n%s\n", s)
			}
		})
	})
}
