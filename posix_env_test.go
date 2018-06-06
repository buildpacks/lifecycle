package lifecycle_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/sclevine/lifecycle"
	"io/ioutil"
	"strings"
)

func TestPOSIXEnv(t *testing.T) {
	spec.Run(t, "POSIX Env", testPOSIXEnv, spec.Report(report.Terminal{}))
}

func testPOSIXEnv(t *testing.T, when spec.G, it spec.S) {
	var (
		env    *lifecycle.POSIXEnv
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
		env = &lifecycle.POSIXEnv{
			Getenv: func(key string) string {
				return result[key]
			},
			Setenv: func(key, value string) error {
				result[key] = strings.Replace(value, tmpDir, "/tmpDir",-1)
				return retErr
			},
			Environ: func() (out []string) {
				for k, v := range result {
					out = append(out, k+"="+v)
				}
				return out
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
				filepath.Join(tmpDir, "include"),
				filepath.Join(tmpDir, "pkgconfig"),
			)
			result = map[string]string{
				"PATH":               "some",
				"LD_LIBRARY_PATH":    "some-ld",
				"LIBRARY_PATH":       "some-library",
				"CPATH":              "some-cpath",
				"C_INCLUDE_PATH":     "some-c-include",
				"CPLUS_INCLUDE_PATH": "cplus-include",
				"OBJC_INCLUDE_PATH":  "objc-include",
				"PKG_CONFIG_PATH":    "pkg-config",
			}
			if err := env.AddRootDir(tmpDir); err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			if !reflect.DeepEqual(result, map[string]string{
				"PATH":               "some:/tmpDir/bin",
				"LD_LIBRARY_PATH":    "some-ld:/tmpDir/lib",
				"LIBRARY_PATH":       "some-library:/tmpDir/lib",
				"CPATH":              "some-cpath:/tmpDir/include",
				"C_INCLUDE_PATH":     "some-c-include:/tmpDir/include",
				"CPLUS_INCLUDE_PATH": "cplus-include:/tmpDir/include",
				"OBJC_INCLUDE_PATH":  "objc-include:/tmpDir/include",
				"PKG_CONFIG_PATH":    "pkg-config:/tmpDir/pkgconfig",
			}) {
				t.Fatalf("Unexpected env: %+v\n", result)
			}
		})

		it("should set POSIX directories on empty POSIX env vars", func() {
			mkdir(t,
				filepath.Join(tmpDir, "bin"),
				filepath.Join(tmpDir, "lib"),
				filepath.Join(tmpDir, "include"),
				filepath.Join(tmpDir, "pkgconfig"),
			)
			if err := env.AddRootDir(tmpDir); err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			if !reflect.DeepEqual(result, map[string]string{
				"PATH":               "/tmpDir/bin",
				"LD_LIBRARY_PATH":    "/tmpDir/lib",
				"LIBRARY_PATH":       "/tmpDir/lib",
				"CPATH":              "/tmpDir/include",
				"C_INCLUDE_PATH":     "/tmpDir/include",
				"CPLUS_INCLUDE_PATH": "/tmpDir/include",
				"OBJC_INCLUDE_PATH":  "/tmpDir/include",
				"PKG_CONFIG_PATH":    "/tmpDir/pkgconfig",
			}) {
				t.Fatalf("Unexpected env: %+v\n", result)
			}
		})
	})
}
