package env_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/env"
)

func TestEnv(t *testing.T) {
	spec.Run(t, "Env", testEnv, spec.Report(report.Terminal{}))
}

func testEnv(t *testing.T, when spec.G, it spec.S) {
	var (
		envv   *env.Env
		tmpDir string
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		envv = &env.Env{
			RootDirMap: map[string][]string{
				"bin": {
					"PATH",
				},
				"lib": {
					"LD_LIBRARY_PATH",
					"LIBRARY_PATH",
				},
			},
			Vars: env.NewVars(map[string]string{}, runtime.GOOS == "windows"),
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#AddRootDir", func() {
		it("should append POSIX directories to existing POSIX env vars", func() {
			mkdir(t,
				filepath.Join(tmpDir, "bin"),
				filepath.Join(tmpDir, "lib"),
			)
			envv.Vars = env.NewVars(map[string]string{
				"PATH":            "some",
				"LD_LIBRARY_PATH": "some-ld",
				"LIBRARY_PATH":    "some-library",
			}, false)
			if err := envv.AddRootDir(tmpDir); err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			out := envv.List()
			sort.Strings(out)

			expected := []string{
				formEnv("LD_LIBRARY_PATH", filepath.Join(tmpDir, "lib"), "some-ld"),
				formEnv("LIBRARY_PATH", filepath.Join(tmpDir, "lib"), "some-library"),
				formEnv("PATH", filepath.Join(tmpDir, "bin"), "some"),
			}

			if s := cmp.Diff(out, expected); s != "" {
				t.Fatalf("Unexpected env:\n%s\n", s)
			}
		})

		it("should set POSIX directories on empty POSIX env vars", func() {
			mkdir(t,
				filepath.Join(tmpDir, "bin"),
				filepath.Join(tmpDir, "lib"),
			)
			if err := envv.AddRootDir(tmpDir); err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			out := envv.List()
			sort.Strings(out)

			expected := []string{
				formEnv("LD_LIBRARY_PATH", filepath.Join(tmpDir, "lib")),
				formEnv("LIBRARY_PATH", filepath.Join(tmpDir, "lib")),
				formEnv("PATH", filepath.Join(tmpDir, "bin")),
			}
			if s := cmp.Diff(out, expected); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})
	})

	when("#AddEnvDir", func() {
		it("should append env vars as filename=file-contents", func() {
			mkdir(t, filepath.Join(tmpDir, "some-dir"))
			mkfile(t, "value-normal", filepath.Join(tmpDir, "VAR_NORMAL"), filepath.Join(tmpDir, "VAR_NORMAL_NEW"))
			mkfile(t, "value-normal-delim", filepath.Join(tmpDir, "VAR_NORMAL_DELIM"), filepath.Join(tmpDir, "VAR_NORMAL_DELIM_NEW"))
			mkfile(t, "[]", filepath.Join(tmpDir, "VAR_NORMAL_DELIM.delim"), filepath.Join(tmpDir, "VAR_NORMAL_DELIM_NEW.delim"))

			mkfile(t, "value-append", filepath.Join(tmpDir, "VAR_APPEND.append"), filepath.Join(tmpDir, "VAR_APPEND_NEW.append"))
			mkfile(t, "value-append-delim", filepath.Join(tmpDir, "VAR_APPEND_DELIM.append"), filepath.Join(tmpDir, "VAR_APPEND_DELIM_NEW.append"))
			mkfile(t, "[]", filepath.Join(tmpDir, "VAR_APPEND_DELIM.delim"), filepath.Join(tmpDir, "VAR_APPEND_DELIM_NEW.delim"))

			mkfile(t, "value-prepend", filepath.Join(tmpDir, "VAR_PREPEND.prepend"), filepath.Join(tmpDir, "VAR_PREPEND_NEW.prepend"))
			mkfile(t, "value-prepend-delim", filepath.Join(tmpDir, "VAR_PREPEND_DELIM.prepend"), filepath.Join(tmpDir, "VAR_PREPEND_DELIM_NEW.prepend"))
			mkfile(t, "[]", filepath.Join(tmpDir, "VAR_PREPEND_DELIM.delim"), filepath.Join(tmpDir, "VAR_PREPEND_DELIM_NEW.delim"))

			mkfile(t, "value-default", filepath.Join(tmpDir, "VAR_DEFAULT.default"), filepath.Join(tmpDir, "VAR_DEFAULT_NEW.default"))
			mkfile(t, "value-override", filepath.Join(tmpDir, "VAR_OVERRIDE.override"), filepath.Join(tmpDir, "VAR_OVERRIDE_NEW.override"))
			mkfile(t, "value-ignore", filepath.Join(tmpDir, "VAR_IGNORE.ignore"))

			envv.Vars = env.NewVars(map[string]string{
				"VAR_NORMAL":        "value-normal-orig",
				"VAR_NORMAL_DELIM":  "value-normal-delim-orig",
				"VAR_APPEND":        "value-append-orig",
				"VAR_APPEND_DELIM":  "value-append-delim-orig",
				"VAR_PREPEND":       "value-prepend-orig",
				"VAR_PREPEND_DELIM": "value-prepend-delim-orig",
				"VAR_DEFAULT":       "value-default-orig",
				"VAR_OVERRIDE":      "value-override-orig",
			}, false)
			if err := envv.AddEnvDir(tmpDir); err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			out := envv.List()
			sort.Strings(out)

			expected := []string{
				formEnv("VAR_APPEND", "value-append-origvalue-append"),
				formEnv("VAR_APPEND_DELIM", "value-append-delim-orig[]value-append-delim"),
				formEnv("VAR_APPEND_DELIM_NEW", "value-append-delim"),
				formEnv("VAR_APPEND_NEW", "value-append"),
				formEnv("VAR_DEFAULT", "value-default-orig"),
				formEnv("VAR_DEFAULT_NEW", "value-default"),
				formEnv("VAR_NORMAL", "value-normal", "value-normal-orig"),
				formEnv("VAR_NORMAL_DELIM", "value-normal-delim[]value-normal-delim-orig"),
				formEnv("VAR_NORMAL_DELIM_NEW", "value-normal-delim"),
				formEnv("VAR_NORMAL_NEW", "value-normal"),
				formEnv("VAR_OVERRIDE", "value-override"),
				formEnv("VAR_OVERRIDE_NEW", "value-override"),
				formEnv("VAR_PREPEND", "value-prependvalue-prepend-orig"),
				formEnv("VAR_PREPEND_DELIM", "value-prepend-delim[]value-prepend-delim-orig"),
				formEnv("VAR_PREPEND_DELIM_NEW", "value-prepend-delim"),
				formEnv("VAR_PREPEND_NEW", "value-prepend"),
			}
			if s := cmp.Diff(out, expected); s != "" {
				t.Fatalf("Unexpected env:\n%s\n", s)
			}
		})
	})

	when("#WithPlatform", func() {
		it("should apply platform env vars as filename=file-contents", func() {
			mkdir(t, filepath.Join(tmpDir, "env", "some-dir"))
			mkfile(t, "value-path", filepath.Join(tmpDir, "env", "PATH"))
			mkfile(t, "value-ld-library-path", filepath.Join(tmpDir, "env", "LD_LIBRARY_PATH"))
			mkfile(t, "value-library-path", filepath.Join(tmpDir, "env", "LIBRARY_PATH"))
			mkfile(t, "value-normal", filepath.Join(tmpDir, "env", "VAR_NORMAL"))
			mkfile(t, "value-override", filepath.Join(tmpDir, "env", "VAR_OVERRIDE"))

			envv.Vars = env.NewVars(map[string]string{
				"VAR_EMPTY":       "",
				"VAR_OVERRIDE":    "value-override-orig",
				"PATH":            "value-path-orig",
				"LD_LIBRARY_PATH": strings.Join([]string{"value-ld-library-path-orig1", "value-ld-library-path-orig2"}, string(os.PathListSeparator)),
			}, false)
			out, err := envv.WithPlatform(tmpDir)
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			sort.Strings(out)

			expected := []string{
				formEnv("LD_LIBRARY_PATH", "value-ld-library-path", "value-ld-library-path-orig1", "value-ld-library-path-orig2"),
				formEnv("LIBRARY_PATH", "value-library-path"),
				formEnv("PATH", "value-path", "value-path-orig"),
				formEnv("VAR_EMPTY", ""),
				formEnv("VAR_NORMAL", "value-normal"),
				formEnv("VAR_OVERRIDE", "value-override"),
			}
			if s := cmp.Diff(out, expected); s != "" {
				t.Fatalf("Unexpected env:\n%s\n", s)
			}
		})
	})

	when("#Get", func() {
		it("should get a value", func() {
			mkdir(t,
				filepath.Join(tmpDir, "bin"),
			)
			envv.Vars = env.NewVars(map[string]string{
				"PATH": "path-orig",
			}, false)
			if err := envv.AddRootDir(tmpDir); err != nil {
				t.Fatalf("Error: %s\n", err)
			}

			expected := strings.Join([]string{filepath.Join(tmpDir, "bin"), "path-orig"}, string(os.PathListSeparator))
			if s := cmp.Diff(envv.Get("PATH"), expected); s != "" {
				t.Fatalf("Unexpected val:\n%s\n", s)
			}
		})
	})
}

func formEnv(name string, values ...string) string {
	return fmt.Sprintf("%s=%s", name, strings.Join(values, string(os.PathListSeparator)))
}

func mkdir(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0777); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func mkfile(t *testing.T, data string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := ioutil.WriteFile(p, []byte(data), 0777); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}
