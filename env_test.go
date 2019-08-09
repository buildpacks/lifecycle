package lifecycle_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
)

func TestEnv(t *testing.T) {
	spec.Run(t, "Env", testEnv, spec.Report(report.Terminal{}))
}

func testEnv(t *testing.T, when spec.G, it spec.S) {
	var (
		env    *lifecycle.Env
		result map[string]string
		retErr error
		tmpDir string
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		result = map[string]string{}
		env = &lifecycle.Env{
			LookupEnv: func(key string) (string, bool) {
				v, ok := result[key]
				return v, ok
			},
			Getenv: func(key string) string {
				return result[key]
			},
			Setenv: func(key, value string) error {
				result[key] = strings.Replace(value, tmpDir, "/tmpDir", -1)
				return retErr
			},
			Unsetenv: func(key string) error {
				delete(result, key)
				return retErr
			},
			Environ: func() (out []string) {
				for k, v := range result {
					out = append(out, k+"="+v)
				}
				return out
			},
			Map: map[string][]string{
				"bin": {
					"PATH",
				},
				"lib": {
					"LD_LIBRARY_PATH",
					"LIBRARY_PATH",
				},
			},
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
			result = map[string]string{
				"PATH":            "some",
				"LD_LIBRARY_PATH": "some-ld",
				"LIBRARY_PATH":    "some-library",
			}
			if err := env.AddRootDir(tmpDir); err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			if s := cmp.Diff(result, map[string]string{
				"PATH":            "/tmpDir/bin:some",
				"LD_LIBRARY_PATH": "/tmpDir/lib:some-ld",
				"LIBRARY_PATH":    "/tmpDir/lib:some-library",
			}); s != "" {
				t.Fatalf("Unexpected env:\n%s\n", s)
			}
		})

		it("should set POSIX directories on empty POSIX env vars", func() {
			mkdir(t,
				filepath.Join(tmpDir, "bin"),
				filepath.Join(tmpDir, "lib"),
			)
			if err := env.AddRootDir(tmpDir); err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			if s := cmp.Diff(result, map[string]string{
				"PATH":            "/tmpDir/bin",
				"LD_LIBRARY_PATH": "/tmpDir/lib",
				"LIBRARY_PATH":    "/tmpDir/lib",
			}); s != "" {
				t.Fatalf("Unexpected env\n%s\n", s)
			}
		})

		it("should return an error when setenv fails", func() {
			retErr = errors.New("some error")
			mkdir(t, filepath.Join(tmpDir, "bin"))
			if err := env.AddRootDir(tmpDir); err != retErr {
				t.Fatalf("Unexpected error: %s\n", err)
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

			result = map[string]string{
				"VAR_NORMAL":        "value-normal-orig",
				"VAR_NORMAL_DELIM":  "value-normal-delim-orig",
				"VAR_APPEND":        "value-append-orig",
				"VAR_APPEND_DELIM":  "value-append-delim-orig",
				"VAR_PREPEND":       "value-prepend-orig",
				"VAR_PREPEND_DELIM": "value-prepend-delim-orig",
				"VAR_DEFAULT":       "value-default-orig",
				"VAR_OVERRIDE":      "value-override-orig",
			}
			if err := env.AddEnvDir(tmpDir); err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			if s := cmp.Diff(result, map[string]string{
				"VAR_NORMAL":            "value-normal:value-normal-orig",
				"VAR_NORMAL_NEW":        "value-normal",
				"VAR_NORMAL_DELIM":      "value-normal-delim[]value-normal-delim-orig",
				"VAR_NORMAL_DELIM_NEW":  "value-normal-delim",
				"VAR_APPEND":            "value-append-origvalue-append",
				"VAR_APPEND_NEW":        "value-append",
				"VAR_APPEND_DELIM":      "value-append-delim-orig[]value-append-delim",
				"VAR_APPEND_DELIM_NEW":  "value-append-delim",
				"VAR_PREPEND":           "value-prependvalue-prepend-orig",
				"VAR_PREPEND_NEW":       "value-prepend",
				"VAR_PREPEND_DELIM":     "value-prepend-delim[]value-prepend-delim-orig",
				"VAR_PREPEND_DELIM_NEW": "value-prepend-delim",
				"VAR_DEFAULT":           "value-default-orig",
				"VAR_DEFAULT_NEW":       "value-default",
				"VAR_OVERRIDE":          "value-override",
				"VAR_OVERRIDE_NEW":      "value-override",
			}); s != "" {
				t.Fatalf("Unexpected env:\n%s\n", s)
			}
		})

		it("should return an error when setenv fails", func() {
			retErr = errors.New("some error")
			mkfile(t, "some-value", filepath.Join(tmpDir, "SOME_VAR"))
			if err := env.AddEnvDir(tmpDir); err != retErr {
				t.Fatalf("Unexpected error: %s\n", err)
			}
		})
	})

	when("#WithPlatform", func() {
		it("should apply platform env vars as filename=file-contents", func() {
			mkdir(t, filepath.Join(tmpDir, "some-dir"))
			mkfile(t, "value-path", filepath.Join(tmpDir, "PATH"))
			mkfile(t, "value-ld-library-path", filepath.Join(tmpDir, "LD_LIBRARY_PATH"))
			mkfile(t, "value-library-path", filepath.Join(tmpDir, "LIBRARY_PATH"))
			mkfile(t, "value-normal", filepath.Join(tmpDir, "VAR_NORMAL"))
			mkfile(t, "value-override", filepath.Join(tmpDir, "VAR_OVERRIDE"))

			result = map[string]string{
				"VAR_EMPTY":       "",
				"VAR_OVERRIDE":    "value-override-orig",
				"PATH":            "value-path-orig",
				"LD_LIBRARY_PATH": "value-ld-library-path-orig1:value-ld-library-path-orig2",
			}
			out, err := env.WithPlatform(tmpDir)
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			sort.Strings(out)
			if s := cmp.Diff(out, []string{
				"LD_LIBRARY_PATH=value-ld-library-path:value-ld-library-path-orig1:value-ld-library-path-orig2",
				"LIBRARY_PATH=value-library-path",
				"PATH=value-path:value-path-orig",
				"VAR_EMPTY=",
				"VAR_NORMAL=value-normal",
				"VAR_OVERRIDE=value-override",
			}); s != "" {
				t.Fatalf("Unexpected env:\n%s\n", s)
			}
			if s := cmp.Diff(result, map[string]string{
				"VAR_EMPTY":       "",
				"VAR_OVERRIDE":    "value-override-orig",
				"PATH":            "value-path-orig",
				"LD_LIBRARY_PATH": "value-ld-library-path-orig1:value-ld-library-path-orig2",
			}); s != "" {
				t.Fatalf("Unexpected env:\n%s\n", s)
			}
		})

		it("should return an error when setenv fails", func() {
			retErr = errors.New("some error")
			mkfile(t, "some-value", filepath.Join(tmpDir, "SOME_VAR"))
			if _, err := env.WithPlatform(tmpDir); err != retErr {
				t.Fatalf("Unexpected error: %s\n", err)
			}
		})
	})
}
