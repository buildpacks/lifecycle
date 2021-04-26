package env_test

import (
	"runtime"
	"sort"
	"testing"

	"github.com/buildpacks/lifecycle/api"

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
		when("platform API >= 0.8", func() {
			var platformAPI *api.Version
			it.Before(func() {
				platformAPI = api.MustParse("0.8")
			})
			it("can include CNB_ASSETS env var", func() {
				benv := env.NewBuildEnv([]string{
					"CNB_STACK_ID=some-stack-id",
					"HOSTNAME=some-hostname",
					"HOME=some-home",
					"HTTPS_PROXY=some-https-proxy",
					"https_proxy=some-https-proxy",
					"HTTP_PROXY=some-http-proxy",
					"http_proxy=some-http-proxy",
					"NO_PROXY=some-no-proxy",
					"no_proxy=some-no-proxy",
					"NOT_INCLUDED=not-included",
					"PATH=some-path",
					"LD_LIBRARY_PATH=some-ld-library-path",
					"LIBRARY_PATH=some-library-path",
					"CPATH=some-cpath",
					"PKG_CONFIG_PATH=some-pkg-config-path",
					"CNB_ASSETS=some-assets-path",
				}, platformAPI)
				out := benv.List()
				sort.Strings(out)
				expectedVars := []string{
					"CNB_ASSETS=some-assets-path",
					"CNB_STACK_ID=some-stack-id",
					"CPATH=some-cpath",
					"HOME=some-home",
					"HOSTNAME=some-hostname",
					"HTTPS_PROXY=some-https-proxy",
					"HTTP_PROXY=some-http-proxy",
					"LD_LIBRARY_PATH=some-ld-library-path",
					"LIBRARY_PATH=some-library-path",
					"NO_PROXY=some-no-proxy",
					"PATH=some-path",
					"PKG_CONFIG_PATH=some-pkg-config-path",
				}
				// Environment variables in Windows are case insensitive, and are added by the lifecycle in uppercase.
				if runtime.GOOS != "windows" {
					expectedVars = append(
						expectedVars,
						"http_proxy=some-http-proxy",
						"https_proxy=some-https-proxy",
						"no_proxy=some-no-proxy",
					)
				}
				if s := cmp.Diff(out, expectedVars); s != "" {
					t.Fatalf("Unexpected env\n%s\n", s)
				}
			})
		})
		when("platform API <0.8", func() {
			var platformAPI *api.Version
			it.Before(func() {
				platformAPI = api.MustParse("0.7")
			})
			it("includes expected vars", func() {
				benv := env.NewBuildEnv([]string{
					"CNB_ASSETS=some-assets-path",
					"CNB_STACK_ID=some-stack-id",
					"HOSTNAME=some-hostname",
					"HOME=some-home",
					"HTTPS_PROXY=some-https-proxy",
					"https_proxy=some-https-proxy",
					"HTTP_PROXY=some-http-proxy",
					"http_proxy=some-http-proxy",
					"NO_PROXY=some-no-proxy",
					"no_proxy=some-no-proxy",
					"NOT_INCLUDED=not-included",
					"PATH=some-path",
					"LD_LIBRARY_PATH=some-ld-library-path",
					"LIBRARY_PATH=some-library-path",
					"CPATH=some-cpath",
					"PKG_CONFIG_PATH=some-pkg-config-path",
				}, platformAPI)
				out := benv.List()
				sort.Strings(out)
				expectedVars := []string{
					"CNB_STACK_ID=some-stack-id",
					"CPATH=some-cpath",
					"HOME=some-home",
					"HOSTNAME=some-hostname",
					"HTTPS_PROXY=some-https-proxy",
					"HTTP_PROXY=some-http-proxy",
					"LD_LIBRARY_PATH=some-ld-library-path",
					"LIBRARY_PATH=some-library-path",
					"NO_PROXY=some-no-proxy",
					"PATH=some-path",
					"PKG_CONFIG_PATH=some-pkg-config-path",
				}
				// Environment variables in Windows are case insensitive, and are added by the lifecycle in uppercase.
				if runtime.GOOS != "windows" {
					expectedVars = append(
						expectedVars,
						"http_proxy=some-http-proxy",
						"https_proxy=some-https-proxy",
						"no_proxy=some-no-proxy",
					)
				}
				if s := cmp.Diff(out, expectedVars); s != "" {
					t.Fatalf("Unexpected env\n%s\n", s)
				}
			})

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
				}, api.MustParse("0.0"))
				out := benv.List()
				h.AssertEq(t, len(out), 1)
				h.AssertEq(t, out[0], "PATH=some-path")
			})
		})

		it("allows keys with '='", func() {
			benv := env.NewBuildEnv([]string{
				"CNB_STACK_ID=included=true",
			}, api.MustParse("0.0"))
			if s := cmp.Diff(benv.List(), []string{
				"CNB_STACK_ID=included=true",
			}); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})

		it("assign the build time root dir map", func() {
			benv := env.NewBuildEnv([]string{}, api.MustParse("0.0"))
			if s := cmp.Diff(benv.RootDirMap, env.POSIXBuildEnv); s != "" {
				t.Fatalf("Unexpected root dir map\n%s\n", s)
			}
		})
	})
}
