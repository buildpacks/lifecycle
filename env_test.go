package lifecycle_test

import (
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

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
			Getenv: func(key string) string {
				return result[key]
			},
			Setenv: func(key, value string) error {
				result[key] = strings.Replace(value, tmpDir, "/tmpDir", -1)
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
			if !reflect.DeepEqual(result, map[string]string{
				"PATH":            "some:/tmpDir/bin",
				"LD_LIBRARY_PATH": "some-ld:/tmpDir/lib",
				"LIBRARY_PATH":    "some-library:/tmpDir/lib",
			}) {
				t.Fatalf("Unexpected env: %+v\n", result)
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
			if !reflect.DeepEqual(result, map[string]string{
				"PATH":            "/tmpDir/bin",
				"LD_LIBRARY_PATH": "/tmpDir/lib",
				"LIBRARY_PATH":    "/tmpDir/lib",
			}) {
				t.Fatalf("Unexpected env: %+v\n", result)
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
			mkfile(t, "some-value-default", filepath.Join(tmpDir, "SOME_VAR_DEFAULT"), filepath.Join(tmpDir, "SOME_VAR_DEFAULT_NEW"))
			mkfile(t, "some-value-append", filepath.Join(tmpDir, "SOME_VAR_APPEND.append"), filepath.Join(tmpDir, "SOME_VAR_APPEND_NEW.append"))
			mkfile(t, "some-value-override", filepath.Join(tmpDir, "SOME_VAR_OVERRIDE.override"), filepath.Join(tmpDir, "SOME_VAR_OVERRIDE_NEW.override"))
			mkfile(t, "some-value-ignore", filepath.Join(tmpDir, "SOME_VAR_IGNORE.ignore"))
			result = map[string]string{
				"SOME_VAR_DEFAULT":  "some-value-default-orig",
				"SOME_VAR_APPEND":   "some-value-append-orig",
				"SOME_VAR_OVERRIDE": "some-value-override-orig",
			}
			if err := env.AddEnvDir(tmpDir); err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			if !reflect.DeepEqual(result, map[string]string{
				"SOME_VAR_DEFAULT":      "some-value-default-orig:some-value-default",
				"SOME_VAR_DEFAULT_NEW":  "some-value-default",
				"SOME_VAR_APPEND":       "some-value-append-origsome-value-append",
				"SOME_VAR_APPEND_NEW":   "some-value-append",
				"SOME_VAR_OVERRIDE":     "some-value-override",
				"SOME_VAR_OVERRIDE_NEW": "some-value-override",
				"SOME_VAR_IGNORE":       "some-value-ignore",
			}) {
				t.Fatalf("Unexpected env: %+v\n", result)
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
}
