package env_test

import (
	"sort"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/env"
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
				"http_proxy=some-http-proxy",
				"https_proxy=some-https-proxy",
				"no_proxy=some-no-proxy",
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
	})
}
