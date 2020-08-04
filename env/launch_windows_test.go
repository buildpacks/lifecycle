package env_test

import (
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"

	"github.com/buildpacks/lifecycle/env"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func testLaunchEnvOS(t *testing.T, when spec.G, it spec.S) {
	when("#NewLaunchEnv", func() {
		when("process dir is the first element in PATH", func() {
			it("strips it", func() {
				lenv := env.NewLaunchEnv([]string{
					`PATH=c:\cnb\process;c:\bin`,
				})
				if s := cmp.Diff(lenv.List(), []string{
					`PATH=c:\bin`,
				}); s != "" {
					t.Fatalf("Unexpected env\n%s\n", s)
				}
			})
		})

		when("launching in Windows", func() {
			it.Before(func() {
				if runtime.GOOS != "windows" {
					t.Skip("This test only applies to Windows launches")
				}
			})

			it("ignores case when initializing", func() {
				benv := env.NewBuildEnv([]string{
					"Path=some-path",
				})
				out := benv.List()
				h.AssertEq(t, len(out), 1)
				h.AssertEq(t, out[0], "PATH=some-path")
			})
		})
	})
}
