// +build linux darwin

package env_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/env"
)

func testLaunchEnvOS(t *testing.T, when spec.G, it spec.S) {
	when("#NewLaunchEnv", func() {
		when("process dir is the first element in PATH", func() {
			it("strips it", func() {
				lenv := env.NewLaunchEnv([]string{
					"PATH=/cnb/process:/usr/bin",
				})
				if s := cmp.Diff(lenv.List(), []string{
					"PATH=/usr/bin",
				}); s != "" {
					t.Fatalf("Unexpected env\n%s\n", s)
				}
			})
		})
	})
}
