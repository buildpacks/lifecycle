package env_test

import (
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/buildpacks/lifecycle/env"
)

func TestLaunchEnv(t *testing.T) {
	t.Run("#NewLaunchEnv", func(t *testing.T) {
		t.Run("excludes vars", func(t *testing.T) {
			lenv := env.NewLaunchEnv([]string{
				"CNB_APP_DIR=excluded",
				"CNB_LAYERS_DIR=excluded",
				"CNB_PROCESS_TYPE=excluded",
				"CNB_PLATFORM_API=excluded",
				"CNB_DEPRECATION_MODE=excluded",
				"CNB_FOO=not-excluded",
			}, "", "")
			if s := cmp.Diff(lenv.List(), []string{
				"CNB_FOO=not-excluded",
			}); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})
		t.Run("path contains process and lifecycle dirs", func(t *testing.T) {
			t.Run("strips them", func(t *testing.T) {
				lenv := env.NewLaunchEnv([]string{
					"PATH=" + strings.Join(
						[]string{"some-process-dir", "some-path", "some-lifecycle-dir"},
						string(os.PathListSeparator),
					),
				}, "some-process-dir", "some-lifecycle-dir")
				if s := cmp.Diff(lenv.List(), []string{
					"PATH=some-path",
				}); s != "" {
					t.Fatalf("Unexpected env\n%s\n", s)
				}
			})
		})
		t.Run("allows keys with '='", func(t *testing.T) {
			lenv := env.NewLaunchEnv([]string{
				"CNB_FOO=some=key",
			}, "", "")
			if s := cmp.Diff(lenv.List(), []string{
				"CNB_FOO=some=key",
			}); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})
		t.Run("assign the Launch time root dir map", func(t *testing.T) {
			lenv := env.NewLaunchEnv([]string{}, "", "")
			if s := cmp.Diff(lenv.RootDirMap, env.POSIXLaunchEnv); s != "" {
				t.Fatalf("Unexpected root dir map\n%s\n", s)
			}
		})
	})
}
