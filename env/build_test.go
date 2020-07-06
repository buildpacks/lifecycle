package env_test

import (
	"runtime"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/env"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestBuildEnv(t *testing.T) {
	spec.Run(t, "BuildEnv", testBuildEnv, spec.Report(report.Terminal{}))
}

func testBuildEnv(t *testing.T, when spec.G, it spec.S) {
	when("#NewBuildEnv", func() {
		it("includes expected vars", func() {
			benv := env.NewBuildEnv([]string{
				"CNB_STACK_ID=some-stack-id",
				"NOT_INCLUDED=not-included",
				"PATH=some-path",
				"LD_LIBRARY_PATH=some-ld-library-path",
				"LIBRARY_PATH=some-library-path",
				"CPATH=some-cpath",
				"PKG_CONFIG_PATH=some-pkg-config-path",
			})
			out := benv.List()
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

		when("building in Windows", func() {
			it.Before(func() {
				if runtime.GOOS != "windows" {
					t.Skip("This test only applies to Windows builds")
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

		it("allows keys with '='", func() {
			benv := env.NewBuildEnv([]string{
				"CNB_STACK_ID=included=true",
			})
			if s := cmp.Diff(benv.List(), []string{
				"CNB_STACK_ID=included=true",
			}); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})

		it("assign the build time root dir map", func() {
			benv := env.NewBuildEnv([]string{})
			if s := cmp.Diff(benv.RootDirMap, env.POSIXBuildEnv); s != "" {
				t.Fatalf("Unexpected root dir map\n%s\n", s)
			}
		})
	})
}
