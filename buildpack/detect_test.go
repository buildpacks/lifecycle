package buildpack_test

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/buildpack/testmock"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDetect(t *testing.T) {
	for _, kind := range []string{buildpack.KindBuildpack, buildpack.KindExtension} {
		spec.Run(t, fmt.Sprintf("Detect-%s", kind), testDetect(kind), spec.Report(report.Terminal{}))
	}
}

func testDetect(kind string) func(t *testing.T, when spec.G, it spec.S) {
	return func(t *testing.T, when spec.G, it spec.S) {
		var (
			mockCtrl     *gomock.Controller
			mockEnv      *testmock.MockBuildEnv
			detectConfig buildpack.DetectConfig
			platformDir  string
			tmpDir       string
			logHandler   *memory.Handler

			someEnv = "ENV_TYPE=some-env"
		)

		it.Before(func() {
			mockCtrl = gomock.NewController(t)
			mockEnv = testmock.NewMockBuildEnv(mockCtrl)

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
				AppDir:      appDir,
				PlatformDir: platformDir,
				Logger:      &log.Logger{Handler: logHandler},
			}
		})

		it.After(func() {
			os.RemoveAll(tmpDir)
			mockCtrl.Finish()
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
			var (
				descriptor *buildpack.Descriptor
			)

			it.Before(func() {
				var descriptorPath string
				switch kind {
				case buildpack.KindBuildpack:
					descriptorPath = filepath.Join("testdata", "by-id", "A", "v1", "buildpack.toml")
				case buildpack.KindExtension:
					descriptorPath = filepath.Join("testdata", "by-id", "extA", "v1", "extension.toml")
				default:
					t.Fatalf("unknown module kind: %s", kind)
				}
				var err error
				descriptor, err = buildpack.ReadDescriptor(descriptorPath)
				h.AssertNil(t, err)
				descriptor.API = api.Buildpack.Latest().String() // override
			})

			when("env", func() {
				when("env type", func() {
					when("clear", func() {
						it("provides a clear env", func() {
							mockEnv.EXPECT().List().Return(append(os.Environ(), "ENV_TYPE=clear"))

							descriptor.Dir += ".clear"        // override
							descriptor.Info().ClearEnv = true // override

							descriptor.Detect(&detectConfig, mockEnv)

							if typ := rdappfile("detect-env-type-A-v1.clear"); typ != "clear" {
								t.Fatalf("Unexpected env type: %s\n", typ)
							}
						})
					})

					when("full", func() {
						it("provides a full env", func() {
							mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "ENV_TYPE=full"), nil)

							descriptor.Detect(&detectConfig, mockEnv)

							if typ := rdappfile("detect-env-type-A-v1"); typ != "full" {
								t.Fatalf("Unexpected env type: %s\n", typ)
							}
						})

						it("errors when the env cannot be found", func() {
							mockEnv.EXPECT().WithPlatform(platformDir).Return(nil, errors.New("some error"))

							detectRun := descriptor.Detect(&detectConfig, mockEnv)

							h.AssertEq(t, detectRun.Code, -1)
							err := detectRun.Err
							if err == nil {
								t.Fatalf("Expected error")
							}
							h.AssertEq(t, err.Error(), `some error`)
						})
					})
				})

				it("sets CNB_BUILDPACK_DIR in the environment", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

					descriptor.Detect(&detectConfig, mockEnv)

					expectedBpDir := descriptor.Dir
					if bpDir := rdappfile("detect-env-cnb-buildpack-dir-A-v1"); bpDir != expectedBpDir {
						t.Fatalf("Unexpected buildpack dir:\n\twanted: %s\n\tgot: %s\n", expectedBpDir, bpDir)
					}
				})

				it("sets CNB_PLATFORM_DIR in the environment", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

					descriptor.Detect(&detectConfig, mockEnv)

					env := rdappfile("detect-env-cnb-platform-dir-A-v1")
					h.AssertEq(t, env, platformDir)
				})

				it("sets CNB_BUILD_PLAN_PATH in the environment", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

					descriptor.Detect(&detectConfig, mockEnv)

					env := rdappfile("detect-env-cnb-build-plan-path-A-v1")
					if env == "unset" {
						t.Fatal("expected CNB_BUILD_PLAN_PATH to be set")
					}
				})
			})

			it("fails and prints the output if the output plan is badly formatted", func() {
				toappfile("\nbad=toml", "detect-plan-A-v1.toml")
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

				detectRun := descriptor.Detect(&detectConfig, mockEnv)

				h.AssertEq(t, detectRun.Code, -1)
				h.AssertStringContains(t, string(detectRun.Output), "detect out: A@v1") // the output from the buildpack detect script
				err := detectRun.Err
				h.AssertEq(t, err.Error(), `toml: line 2 (last key "bad"): expected value but found "toml" instead`)
			})

			when("plan deprecations", func() {
				it.Before(func() {
					h.SkipIf(t, kind == buildpack.KindExtension, "extensions do not output requires")
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)
				})

				it("fails if the plan has both a top level version and a metadata version", func() {
					toappfile("\n[[requires]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
					toappfile("\n[requires.metadata]\n version = \"some-version\"", "detect-plan-A-v1.toml")

					detectRun := descriptor.Detect(&detectConfig, mockEnv)

					h.AssertEq(t, detectRun.Code, -1)
					err := detectRun.Err
					if err == nil {
						t.Fatalf("Expected error")
					}
					h.AssertEq(t, err.Error(), strings.ToLower(kind)+` A has a "version" key and a "metadata.version" which cannot be specified together. "metadata.version" should be used instead`)
				})

				it("fails if there is an alternate plan with both a top level version and a metadata version", func() {
					toappfile("\n[[provides]]\n name = \"dep2-missing\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or]]", "detect-plan-A-v1.toml")
					toappfile("\n[[or.provides]]\n name = \"dep1-present\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or.requires]]\n name = \"dep1-present\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
					toappfile("\n[or.requires.metadata]\n version = \"some-version\"", "detect-plan-A-v1.toml")

					detectRun := descriptor.Detect(&detectConfig, mockEnv)

					h.AssertEq(t, detectRun.Code, -1)
					err := detectRun.Err
					if err == nil {
						t.Fatalf("Expected error")
					}
					h.AssertEq(t, err.Error(), strings.ToLower(kind)+` A has a "version" key and a "metadata.version" which cannot be specified together. "metadata.version" should be used instead`)
				})

				it("warns if the plan has a top level version", func() {
					toappfile("\n[[requires]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-A-v1.toml")

					detectRun := descriptor.Detect(&detectConfig, mockEnv)

					h.AssertEq(t, detectRun.Code, 0)
					err := detectRun.Err
					if err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := h.AllLogs(logHandler); !strings.Contains(s,
						strings.ToLower(kind)+` A has a "version" key. This key is deprecated in build plan requirements in buildpack API 0.3. "metadata.version" should be used instead`,
					) {
						t.Fatalf("Expected log to contain warning:\n%s\n", s)
					}
				})

				it("warns if there is an alternate plan with a top level version", func() {
					toappfile("\n[[provides]]\n name = \"dep2-missing\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or]]", "detect-plan-A-v1.toml")
					toappfile("\n[[or.provides]]\n name = \"dep1-present\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or.requires]]\n name = \"dep1-present\"\n version = \"some-version\"", "detect-plan-A-v1.toml")

					detectRun := descriptor.Detect(&detectConfig, mockEnv)

					h.AssertEq(t, detectRun.Code, 0)
					err := detectRun.Err
					if err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := h.AllLogs(logHandler); !strings.Contains(s,
						strings.ToLower(kind)+` A has a "version" key. This key is deprecated in build plan requirements in buildpack API 0.3. "metadata.version" should be used instead`,
					) {
						t.Fatalf("Expected log to contain warning:\n%s\n", s)
					}
				})
			})

			when("extensions", func() {
				it.Before(func() {
					h.SkipIf(t, kind == buildpack.KindBuildpack, "")
				})

				it("fails if the plan has requires", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

					toappfile("\n[[requires]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-A-v1.toml")

					detectRun := descriptor.Detect(&detectConfig, mockEnv)

					h.AssertEq(t, detectRun.Code, -1)
					err := detectRun.Err
					h.AssertEq(t, err.Error(), `extension A outputs "requires" which is not allowed`)
				})

				it("fails if there is an alternate plan with requires", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

					toappfile("\n[[provides]]\n name = \"some-dep\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or]]", "detect-plan-A-v1.toml")
					toappfile("\n[[or.requires]]\n name = \"other-dep\"", "detect-plan-A-v1.toml")

					detectRun := descriptor.Detect(&detectConfig, mockEnv)

					h.AssertEq(t, detectRun.Code, -1)
					err := detectRun.Err
					h.AssertEq(t, err.Error(), `extension A outputs "requires" which is not allowed`)
				})

				when("/bin/detect is missing", func() {
					it.Before(func() {
						var err error
						descriptor, err = buildpack.ReadDescriptor(filepath.Join("testdata", "by-id", "extB", "v1", "extension.toml"))
						h.AssertNil(t, err)
						descriptor.API = api.Buildpack.Latest().String() // override
					})

					it("passes detection", func() {
						detectRun := descriptor.Detect(&detectConfig, mockEnv)
						h.AssertEq(t, detectRun.Code, 0)

						t.Log("treats the extension root as a pre-populated output directory")
						h.AssertEq(t, detectRun.Provides, []buildpack.Provide{{Name: "some-dep"}})
					})

					when("plan is missing", func() {
						it.Before(func() {
							var err error
							descriptor, err = buildpack.ReadDescriptor(filepath.Join("testdata", "by-id", "extC", "v1", "extension.toml"))
							h.AssertNil(t, err)
							descriptor.API = api.Buildpack.Latest().String() // override
						})

						it("passes detection", func() {
							detectRun := descriptor.Detect(&detectConfig, mockEnv)
							h.AssertEq(t, detectRun.Code, 0)

							t.Log("treats the extension root as a pre-populated output directory")
							var empty []buildpack.Provide
							h.AssertEq(t, detectRun.Provides, empty)
						})
					})
				})
			})

			when("buildpack api < 0.8", func() {
				it.Before(func() {
					h.SkipIf(t, kind == buildpack.KindExtension, "")
					descriptor.API = "0.7"
				})

				it("does not set environment variables for positional arguments", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

					descriptor.Detect(&detectConfig, mockEnv)

					for _, file := range []string{
						"detect-env-cnb-platform-dir-A-v1",
						"detect-env-cnb-build-plan-path-A-v1",
					} {
						contents := rdappfile(file)
						if contents != "unset" {
							t.Fatalf("Expected %s to be unset; got %s", file, contents)
						}
					}
				})
			})

			when("buildpack api = 0.2", func() {
				it.Before(func() {
					h.SkipIf(t, kind == buildpack.KindExtension, "")
					descriptor.API = "0.2"
				})

				it("fails if the plan has a top level version and a metadata version that are different", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

					toappfile("\n[[provides]]\n name = \"dep2\"", "detect-plan-A-v1.toml")
					toappfile("\n[[requires]]\n name = \"dep1\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
					toappfile("\n[requires.metadata]\n version = \"some-other-version\"", "detect-plan-A-v1.toml")

					detectRun := descriptor.Detect(&detectConfig, mockEnv)

					h.AssertEq(t, detectRun.Code, -1)
					err := detectRun.Err
					if err == nil {
						t.Fatalf("Expected error")
					}
					h.AssertEq(t, err.Error(), strings.ToLower(kind)+` A has a "version" key that does not match "metadata.version"`)
				})

				it("fails if there is an alternate plan with a top level version and a metadata version that are different", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

					toappfile("\n[[requires]]\n name = \"dep3-missing\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or]]", "detect-plan-A-v1.toml")
					toappfile("\n[[or.requires]]\n name = \"dep1-present\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
					toappfile("\n[or.requires.metadata]\n version = \"some-other-version\"", "detect-plan-A-v1.toml")

					detectRun := descriptor.Detect(&detectConfig, mockEnv)

					h.AssertEq(t, detectRun.Code, -1)
					err := detectRun.Err
					if err == nil {
						t.Fatalf("Expected error")
					}
					h.AssertEq(t, err.Error(), strings.ToLower(kind)+` A has a "version" key that does not match "metadata.version"`)
				})
			})
		})
	}
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
