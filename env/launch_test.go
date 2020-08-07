package env_test

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestLaunchEnv(t *testing.T) {
	spec.Run(t, "LaunchEnv", testLaunchEnv, spec.Report(report.Terminal{}))
}

func testLaunchEnv(t *testing.T, when spec.G, it spec.S) {
	when("#NewLaunchEnv", func() {
		it("excludes vars", func() {
			lenv := env.NewLaunchEnv([]string{
				"CNB_APP_DIR=excluded",
				"CNB_LAYERS_DIR=excluded",
				"CNB_PROCESS_TYPE=excluded",
				"CNB_PLATFORM_API=excluded",
				"CNB_DEPRECATION_MODE=excluded",
				"CNB_FOO=not-excluded",
			}, "")
			if s := cmp.Diff(lenv.List(), []string{
				"CNB_FOO=not-excluded",
			}); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})

		when("#NewLaunchEnv", func() {
			when("process dir is the first element in PATH", func() {
				it("strips it", func() {
					lenv := env.NewLaunchEnv([]string{
						"PATH=" + strings.Join([]string{"some-process-dir", "some-path"}, string(os.PathListSeparator)),
					}, "some-process-dir")
					if s := cmp.Diff(lenv.List(), []string{
						"PATH=some-path",
					}); s != "" {
						t.Fatalf("Unexpected env\n%s\n", s)
					}
				})
			})
		})

		it("allows keys with '='", func() {
			lenv := env.NewLaunchEnv([]string{
				"CNB_FOO=some=key",
			}, launch.ProcessDir)
			if s := cmp.Diff(lenv.List(), []string{
				"CNB_FOO=some=key",
			}); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})

		it("assign the Launch time root dir map", func() {
			lenv := env.NewLaunchEnv([]string{}, launch.ProcessDir)
			if s := cmp.Diff(lenv.RootDirMap, env.POSIXLaunchEnv); s != "" {
				t.Fatalf("Unexpected root dir map\n%s\n", s)
			}
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
