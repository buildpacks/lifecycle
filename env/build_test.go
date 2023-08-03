package env_test

import (
	"runtime"
	"sort"
	"testing"

	"github.com/golang/mock/gomock"
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
	var (
		mockController *gomock.Controller
	)

	it.Before(func() {
		mockController = gomock.NewController(t)
	})

	it.After(func() {
		mockController.Finish()
	})

	when("#NewBuildEnv", func() {
		it("includes expected vars", func() {
			benv := env.NewBuildEnv([]string{
				"CNB_STACK_ID=some-stack-id",
				"CNB_TARGET_ARCH=st-louis",
				"CNB_TARGET_ARCH_VARIANT=suburban",
				"CNB_TARGET_OS=BeOS",
				"CNB_TARGET_DISTRO_NAME=web",
				"CNB_TARGET_DISTRO_VERSION=3.0",
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
			})
			out := benv.List()
			sort.Strings(out)
			expectedVars := []string{
				"CNB_STACK_ID=some-stack-id",
				"CNB_TARGET_ARCH=st-louis",
				"CNB_TARGET_ARCH_VARIANT=suburban",
				"CNB_TARGET_DISTRO_NAME=web",
				"CNB_TARGET_DISTRO_VERSION=3.0",
				"CNB_TARGET_OS=BeOS",
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
	})
}
