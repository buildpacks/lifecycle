package buildpack_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDetect(t *testing.T) {
	spec.Run(t, "Detect", testDetect, spec.Report(report.Terminal{}))
}

func testDetect(t *testing.T, when spec.G, it spec.S) {
	var (
		detectConfig buildpack.DetectConfig
		platformDir  string
		tmpDir       string
		logHandler   *memory.Handler
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		platformDir = filepath.Join(tmpDir, "platform")
		appDir := filepath.Join(tmpDir, "app")
		h.Mkdir(t, appDir, filepath.Join(platformDir, "env"))

		logHandler = memory.New()

		detectConfig = buildpack.DetectConfig{
			FullEnv:     append(os.Environ(), "ENV_TYPE=full"),
			ClearEnv:    append(os.Environ(), "ENV_TYPE=clear"),
			AppDir:      appDir,
			PlatformDir: platformDir,
			Logger:      &log.Logger{Handler: logHandler},
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	toappfile := func(data string, paths ...string) {
		t.Helper()
		for _, p := range paths {
			tofile(t, data, filepath.Join(detectConfig.AppDir, p))
		}
	}
	rdappfile := func(path string) string {
		t.Helper()
		return h.Rdfile(t, filepath.Join(detectConfig.AppDir, path))
	}

	when("#Detect", func() {
		var bpTOML buildpack.Descriptor

		it.Before(func() {
			bpPath, err := filepath.Abs(filepath.Join("testdata", "by-id", "A", "v1"))
			h.AssertNil(t, err)

			bpTOML = buildpack.Descriptor{
				API: api.Buildpack.Latest().String(),
				Buildpack: buildpack.Info{
					ID: "A",
				},
				Dir: bpPath,
			}
		})

		when("env type", func() {
			when("clear", func() {
				it("should select an appropriate env type", func() {
					bpPath, err := filepath.Abs(filepath.Join("testdata", "by-id", "A", "v1.clear"))
					h.AssertNil(t, err)
					bpTOML.Dir = bpPath
					bpTOML.Buildpack.ClearEnv = true

					bpTOML.Detect(&detectConfig)

					if typ := rdappfile("detect-env-type-A-v1.clear"); typ != "clear" {
						t.Fatalf("Unexpected env type: %s\n", typ)
					}
				})
			})

			when("full", func() {
				it("should select an appropriate env type", func() {
					bpTOML.Detect(&detectConfig)

					if typ := rdappfile("detect-env-type-A-v1"); typ != "full" {
						t.Fatalf("Unexpected env type: %s\n", typ)
					}
				})
			})
		})

		it("should set CNB_BUILDPACK_DIR in the environment", func() {
			bpTOML.Detect(&detectConfig)

			expectedBpDir := bpTOML.Dir
			if bpDir := rdappfile("detect-env-cnb-buildpack-dir-A-v1"); bpDir != expectedBpDir {
				t.Fatalf("Unexpected buildpack dir:\n\twanted: %s\n\tgot: %s\n", expectedBpDir, bpDir)
			}
		})

		it("should fail and print the output if the buildpack plan file has a bad format", func() {
			mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

			toappfile("\nbad=toml", "detect-plan-A-v1.toml")

			detectRun := bpTOML.Detect(&detectConfig, mockEnv)

			h.AssertEq(t, detectRun.Code, -1)
			h.AssertStringContains(t, string(detectRun.Output), "detect out: A@v1\ndetect err: A@v1") // the output from the buildpack detect script
			err := detectRun.Err
			h.AssertEq(t, err.Error(), `Near line 2 (last key parsed 'bad'): expected value but found "toml" instead`)
		})

		it("should fail if buildpacks have both a top level version and a metadata version", func() {
			toappfile("\n[[requires]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
			toappfile("\n[requires.metadata]\n version = \"some-version\"", "detect-plan-A-v1.toml")

			detectRun := bpTOML.Detect(&detectConfig)

			h.AssertEq(t, detectRun.Code, -1)
			err := detectRun.Err
			if err == nil {
				t.Fatalf("Expected error")
			}
			h.AssertEq(t, err.Error(), `buildpack A has a "version" key and a "metadata.version" which cannot be specified together. "metadata.version" should be used instead`)
		})

		it("should fail if buildpack has alternate build plan with both a top level version and a metadata version", func() {
			toappfile("\n[[provides]]\n name = \"dep2-missing\"", "detect-plan-A-v1.toml")
			toappfile("\n[[or]]", "detect-plan-A-v1.toml")
			toappfile("\n[[or.provides]]\n name = \"dep1-present\"", "detect-plan-A-v1.toml")
			toappfile("\n[[or.requires]]\n name = \"dep1-present\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
			toappfile("\n[or.requires.metadata]\n version = \"some-version\"", "detect-plan-A-v1.toml")

			detectRun := bpTOML.Detect(&detectConfig)

			h.AssertEq(t, detectRun.Code, -1)
			err := detectRun.Err
			if err == nil {
				t.Fatalf("Expected error")
			}
			h.AssertEq(t, err.Error(), `buildpack A has a "version" key and a "metadata.version" which cannot be specified together. "metadata.version" should be used instead`)
		})

		it("should warn if buildpacks have a top level version", func() {
			toappfile("\n[[requires]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-A-v1.toml")

			detectRun := bpTOML.Detect(&detectConfig)

			h.AssertEq(t, detectRun.Code, 0)
			err := detectRun.Err
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}
			if s := h.AllLogs(logHandler); !strings.Contains(s,
				`Warning: buildpack A has a "version" key. This key is deprecated in build plan requirements in buildpack API 0.3. "metadata.version" should be used instead`,
			) {
				t.Fatalf("Expected log to contain warning:\n%s\n", s)
			}
		})

		it("should warn if buildpack has alternate build plan with a top level version", func() {
			toappfile("\n[[provides]]\n name = \"dep2-missing\"", "detect-plan-A-v1.toml")
			toappfile("\n[[or]]", "detect-plan-A-v1.toml")
			toappfile("\n[[or.provides]]\n name = \"dep1-present\"", "detect-plan-A-v1.toml")
			toappfile("\n[[or.requires]]\n name = \"dep1-present\"\n version = \"some-version\"", "detect-plan-A-v1.toml")

			detectRun := bpTOML.Detect(&detectConfig)

			h.AssertEq(t, detectRun.Code, 0)
			err := detectRun.Err
			if err != nil {
				t.Fatalf("Unexpected error:\n%s\n", err)
			}
			if s := h.AllLogs(logHandler); !strings.Contains(s,
				`Warning: buildpack A has a "version" key. This key is deprecated in build plan requirements in buildpack API 0.3. "metadata.version" should be used instead`,
			) {
				t.Fatalf("Expected log to contain warning:\n%s\n", s)
			}
		})

		when("buildpack api = 0.2", func() {
			it.Before(func() {
				bpTOML.API = "0.2"
			})

			it("should fail if buildpacks have a top level version and a metadata version that are different", func() {
				toappfile("\n[[provides]]\n name = \"dep2\"", "detect-plan-A-v1.toml")
				toappfile("\n[[requires]]\n name = \"dep1\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
				toappfile("\n[requires.metadata]\n version = \"some-other-version\"", "detect-plan-A-v1.toml")

				detectRun := bpTOML.Detect(&detectConfig)

				h.AssertEq(t, detectRun.Code, -1)
				err := detectRun.Err
				if err == nil {
					t.Fatalf("Expected error")
				}
				h.AssertEq(t, err.Error(), `buildpack A has a "version" key that does not match "metadata.version"`)
			})

			it("should fail if buildpack has alternate build plan with a top level version and a metadata version that are different", func() {
				toappfile("\n[[requires]]\n name = \"dep3-missing\"", "detect-plan-A-v1.toml")
				toappfile("\n[[or]]", "detect-plan-A-v1.toml")
				toappfile("\n[[or.requires]]\n name = \"dep1-present\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
				toappfile("\n[or.requires.metadata]\n version = \"some-other-version\"", "detect-plan-A-v1.toml")

				detectRun := bpTOML.Detect(&detectConfig)

				h.AssertEq(t, detectRun.Code, -1)
				err := detectRun.Err
				if err == nil {
					t.Fatalf("Expected error")
				}
				h.AssertEq(t, err.Error(), `buildpack A has a "version" key that does not match "metadata.version"`)
			})
		})
	})
}

func tofile(t *testing.T, data string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0777)
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		if _, err := f.Write([]byte(data)); err != nil {
			f.Close()
			t.Fatalf("Error: %s\n", err)
		}
		f.Close()
	}
}
