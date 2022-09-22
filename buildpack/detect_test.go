package buildpack_test

import (
	"errors"
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
	llog "github.com/buildpacks/lifecycle/log"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestDetect(t *testing.T) {
	spec.Run(t, "unit-detect", testDetect, spec.Report(report.Terminal{}))
}

func testDetect(t *testing.T, when spec.G, it spec.S) {
	var (
		mockCtrl *gomock.Controller
		executor *buildpack.DefaultDetectExecutor

		// detect inputs
		inputs      buildpack.DetectInputs
		tmpDir      string
		platformDir string
		mockEnv     *testmock.MockBuildEnv

		someEnv = "ENV_TYPE=some-env"

		logger     llog.Logger
		logHandler = memory.New()
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		executor = &buildpack.DefaultDetectExecutor{}

		// setup dirs
		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		appDir := filepath.Join(tmpDir, "app")
		platformDir = filepath.Join(tmpDir, "platform")
		h.Mkdir(t, appDir, filepath.Join(platformDir, "env"))

		// make inputs
		mockEnv = testmock.NewMockBuildEnv(mockCtrl)
		inputs = buildpack.DetectInputs{
			AppDir:      appDir,
			PlatformDir: platformDir,
			Env:         mockEnv,
		}

		logger = &log.Logger{Handler: logHandler}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
		mockCtrl.Finish()
	})

	toappfile := func(data string, paths ...string) {
		t.Helper()
		for _, p := range paths {
			tofile(t, data, filepath.Join(inputs.AppDir, p))
		}
	}
	rdappfile := func(path string) string {
		t.Helper()
		return h.Rdfile(t, filepath.Join(inputs.AppDir, path))
	}

	when("#Detect", func() {
		when("for buildpack", func() {
			var (
				descriptor     *buildpack.BpDescriptor
				descriptorPath string
			)

			it.Before(func() {
				descriptorPath = filepath.Join("testdata", "buildpack", "by-id", "A", "v1", "buildpack.toml")
				var err error
				descriptor, err = buildpack.ReadBpDescriptor(descriptorPath)
				h.AssertNil(t, err)
				descriptor.WithAPI = api.Buildpack.Latest().String() // override
			})

			when("env", func() {
				when("clear", func() {
					it("provides a clear env", func() {
						mockEnv.EXPECT().List().Return(append(os.Environ(), "ENV_TYPE=clear"))

						descriptor.WithRootDir += ".clear"   // override
						descriptor.Buildpack.ClearEnv = true // override

						executor.Detect(descriptor, inputs, logger)

						if typ := rdappfile("detect-env-type-A-v1.clear"); typ != "clear" {
							t.Fatalf("Unexpected env type: %s\n", typ)
						}
					})

					it("sets CNB_vars", func() {
						mockEnv.EXPECT().List().Return(append(os.Environ(), "ENV_TYPE=clear"))

						descriptor.WithRootDir += ".clear"   // override
						descriptor.Buildpack.ClearEnv = true // override

						executor.Detect(descriptor, inputs, logger)

						var actual string
						t.Log("sets CNB_BUILDPACK_DIR")
						actual = rdappfile("detect-env-cnb-buildpack-dir-A-v1.clear")
						h.AssertEq(t, actual, descriptor.WithRootDir)

						t.Log("sets CNB_PLATFORM_DIR")
						actual = rdappfile("detect-env-cnb-platform-dir-A-v1.clear")
						h.AssertEq(t, actual, platformDir)

						t.Log("sets CNB_BUILD_PLAN_PATH")
						actual = rdappfile("detect-env-cnb-build-plan-path-A-v1.clear")
						if isUnset(actual) {
							t.Fatal("expected CNB_BUILD_PLAN_PATH to be set")
						}
					})
				})

				when("full", func() {
					it("provides a full env", func() {
						mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "ENV_TYPE=full"), nil)

						executor.Detect(descriptor, inputs, logger)

						if typ := rdappfile("detect-env-type-A-v1"); typ != "full" {
							t.Fatalf("Unexpected env type: %s\n", typ)
						}
					})

					it("sets CNB_vars", func() {
						mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

						executor.Detect(descriptor, inputs, logger)

						var actual string
						t.Log("sets CNB_BUILDPACK_DIR")
						actual = rdappfile("detect-env-cnb-buildpack-dir-A-v1")
						h.AssertEq(t, actual, descriptor.WithRootDir)

						t.Log("sets CNB_PLATFORM_DIR")
						actual = rdappfile("detect-env-cnb-platform-dir-A-v1")
						h.AssertEq(t, actual, platformDir)

						t.Log("sets CNB_BUILD_PLAN_PATH")
						actual = rdappfile("detect-env-cnb-build-plan-path-A-v1")
						if isUnset(actual) {
							t.Fatal("expected CNB_BUILD_PLAN_PATH to be set")
						}
					})

					it("errors when <platform>/env cannot be loaded", func() {
						mockEnv.EXPECT().WithPlatform(platformDir).Return(nil, errors.New("some error"))

						detectRun := executor.Detect(descriptor, inputs, logger)

						h.AssertEq(t, detectRun.Code, -1)
						err := detectRun.Err
						if err == nil {
							t.Fatalf("Expected error")
						}
						h.AssertEq(t, err.Error(), `some error`)
					})
				})
			})

			it("errors and prints the output if the output plan is badly formatted", func() {
				toappfile("\nbad=toml", "detect-plan-A-v1.toml")
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

				detectRun := executor.Detect(descriptor, inputs, logger)

				h.AssertEq(t, detectRun.Code, -1)
				h.AssertStringContains(t, string(detectRun.Output), "detect out: A@v1") // the output from the buildpack detect script
				err := detectRun.Err
				h.AssertEq(t, err.Error(), `toml: line 2 (last key "bad"): expected value but found "toml" instead`)
			})

			when("plan deprecations", func() {
				it.Before(func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)
				})

				it("errors if the plan has both a top level version and a metadata version", func() {
					toappfile("\n[[requires]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
					toappfile("\n[requires.metadata]\n version = \"some-version\"", "detect-plan-A-v1.toml")

					detectRun := executor.Detect(descriptor, inputs, logger)

					h.AssertEq(t, detectRun.Code, -1)
					err := detectRun.Err
					if err == nil {
						t.Fatalf("Expected error")
					}
					h.AssertEq(t, err.Error(), `buildpack A has a "version" key and a "metadata.version" which cannot be specified together. "metadata.version" should be used instead`)
				})

				it("errors if there is an alternate plan with both a top level version and a metadata version", func() {
					toappfile("\n[[provides]]\n name = \"dep2-missing\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or]]", "detect-plan-A-v1.toml")
					toappfile("\n[[or.provides]]\n name = \"dep1-present\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or.requires]]\n name = \"dep1-present\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
					toappfile("\n[or.requires.metadata]\n version = \"some-version\"", "detect-plan-A-v1.toml")

					detectRun := executor.Detect(descriptor, inputs, logger)

					h.AssertEq(t, detectRun.Code, -1)
					err := detectRun.Err
					if err == nil {
						t.Fatalf("Expected error")
					}
					h.AssertEq(t, err.Error(), `buildpack A has a "version" key and a "metadata.version" which cannot be specified together. "metadata.version" should be used instead`)
				})

				it("warns if the plan has a top level version", func() {
					toappfile("\n[[requires]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-A-v1.toml")

					detectRun := executor.Detect(descriptor, inputs, logger)

					h.AssertEq(t, detectRun.Code, 0)
					err := detectRun.Err
					if err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := h.AllLogs(logHandler); !strings.Contains(s,
						`buildpack A has a "version" key. This key is deprecated in build plan requirements in buildpack API 0.3. "metadata.version" should be used instead`,
					) {
						t.Fatalf("Expected log to contain warning:\n%s\n", s)
					}
				})

				it("warns if there is an alternate plan with a top level version", func() {
					toappfile("\n[[provides]]\n name = \"dep2-missing\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or]]", "detect-plan-A-v1.toml")
					toappfile("\n[[or.provides]]\n name = \"dep1-present\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or.requires]]\n name = \"dep1-present\"\n version = \"some-version\"", "detect-plan-A-v1.toml")

					detectRun := executor.Detect(descriptor, inputs, logger)

					h.AssertEq(t, detectRun.Code, 0)
					err := detectRun.Err
					if err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := h.AllLogs(logHandler); !strings.Contains(s,
						`buildpack A has a "version" key. This key is deprecated in build plan requirements in buildpack API 0.3. "metadata.version" should be used instead`,
					) {
						t.Fatalf("Expected log to contain warning:\n%s\n", s)
					}
				})
			})

			when("buildpack api < 0.8", func() {
				it.Before(func() {
					descriptor.WithAPI = "0.7"
				})

				it("does not set environment variables for positional arguments", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

					executor.Detect(descriptor, inputs, logger)

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
					descriptor.WithAPI = "0.2"
				})

				it("errors if the plan has a top level version and a metadata version that are different", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

					toappfile("\n[[provides]]\n name = \"dep2\"", "detect-plan-A-v1.toml")
					toappfile("\n[[requires]]\n name = \"dep1\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
					toappfile("\n[requires.metadata]\n version = \"some-other-version\"", "detect-plan-A-v1.toml")

					detectRun := executor.Detect(descriptor, inputs, logger)

					h.AssertEq(t, detectRun.Code, -1)
					err := detectRun.Err
					if err == nil {
						t.Fatalf("Expected error")
					}
					h.AssertEq(t, err.Error(), `buildpack A has a "version" key that does not match "metadata.version"`)
				})

				it("errors if there is an alternate plan with a top level version and a metadata version that are different", func() {
					mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

					toappfile("\n[[requires]]\n name = \"dep3-missing\"", "detect-plan-A-v1.toml")
					toappfile("\n[[or]]", "detect-plan-A-v1.toml")
					toappfile("\n[[or.requires]]\n name = \"dep1-present\"\n version = \"some-version\"", "detect-plan-A-v1.toml")
					toappfile("\n[or.requires.metadata]\n version = \"some-other-version\"", "detect-plan-A-v1.toml")

					detectRun := executor.Detect(descriptor, inputs, logger)

					h.AssertEq(t, detectRun.Code, -1)
					err := detectRun.Err
					if err == nil {
						t.Fatalf("Expected error")
					}
					h.AssertEq(t, err.Error(), `buildpack A has a "version" key that does not match "metadata.version"`)
				})
			})
		})

		when("for extension", func() {
			var (
				descriptor     *buildpack.ExtDescriptor
				descriptorPath string
			)

			it.Before(func() {
				descriptorPath = filepath.Join("testdata", "extension", "by-id", "A", "v1", "extension.toml")
				var err error
				descriptor, err = buildpack.ReadExtDescriptor(descriptorPath)
				h.AssertNil(t, err)
				descriptor.WithAPI = api.Buildpack.Latest().String() // override
			})

			when("env", func() {
				when("clear", func() {
					it("provides a clear env", func() {
						mockEnv.EXPECT().List().Return(append(os.Environ(), "ENV_TYPE=clear"))

						descriptor.WithRootDir += ".clear"   // override
						descriptor.Extension.ClearEnv = true // override

						executor.Detect(descriptor, inputs, logger)

						if typ := rdappfile("detect-env-type-A-v1.clear"); typ != "clear" {
							t.Fatalf("Unexpected env type: %s\n", typ)
						}
					})

					it("sets CNB_vars", func() {
						mockEnv.EXPECT().List().Return(append(os.Environ(), "ENV_TYPE=clear"))

						descriptor.WithRootDir += ".clear"   // override
						descriptor.Extension.ClearEnv = true // override

						executor.Detect(descriptor, inputs, logger)

						var actual string
						t.Log("sets CNB_EXTENSION_DIR")
						actual = rdappfile("detect-env-cnb-extension-dir-A-v1.clear")
						h.AssertEq(t, actual, descriptor.WithRootDir)

						t.Log("sets CNB_PLATFORM_DIR")
						actual = rdappfile("detect-env-cnb-platform-dir-A-v1.clear")
						h.AssertEq(t, actual, platformDir)

						t.Log("sets CNB_BUILD_PLAN_PATH")
						actual = rdappfile("detect-env-cnb-build-plan-path-A-v1.clear")
						if isUnset(actual) {
							t.Fatal("expected CNB_BUILD_PLAN_PATH to be set")
						}
					})
				})

				when("full", func() {
					it("provides a full env", func() {
						mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "ENV_TYPE=full"), nil)

						executor.Detect(descriptor, inputs, logger)

						if typ := rdappfile("detect-env-type-A-v1"); typ != "full" {
							t.Fatalf("Unexpected env type: %s\n", typ)
						}
					})

					it("sets CNB_vars", func() {
						mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

						executor.Detect(descriptor, inputs, logger)

						var actual string
						t.Log("sets CNB_EXTENSION_DIR")
						actual = rdappfile("detect-env-cnb-extension-dir-A-v1")
						h.AssertEq(t, actual, descriptor.WithRootDir)

						t.Log("sets CNB_PLATFORM_DIR")
						actual = rdappfile("detect-env-cnb-platform-dir-A-v1")
						h.AssertEq(t, actual, platformDir)

						t.Log("sets CNB_BUILD_PLAN_PATH")
						actual = rdappfile("detect-env-cnb-build-plan-path-A-v1")
						if isUnset(actual) {
							t.Fatal("expected CNB_BUILD_PLAN_PATH to be set")
						}
					})

					it("errors when <platform>/env cannot be loaded", func() {
						mockEnv.EXPECT().WithPlatform(platformDir).Return(nil, errors.New("some error"))

						detectRun := executor.Detect(descriptor, inputs, logger)

						h.AssertEq(t, detectRun.Code, -1)
						err := detectRun.Err
						if err == nil {
							t.Fatalf("Expected error")
						}
						h.AssertEq(t, err.Error(), `some error`)
					})
				})
			})

			it("errors and prints the output if the output plan is badly formatted", func() {
				toappfile("\nbad=toml", "detect-plan-A-v1.toml")
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

				detectRun := executor.Detect(descriptor, inputs, logger)

				h.AssertEq(t, detectRun.Code, -1)
				h.AssertStringContains(t, string(detectRun.Output), "detect out: A@v1") // the output from the buildpack detect script
				err := detectRun.Err
				h.AssertEq(t, err.Error(), `toml: line 2 (last key "bad"): expected value but found "toml" instead`)
			})

			it("errors if the plan has requires", func() {
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

				toappfile("\n[[requires]]\n name = \"dep2\"\n version = \"some-version\"", "detect-plan-A-v1.toml")

				detectRun := executor.Detect(descriptor, inputs, logger)

				h.AssertEq(t, detectRun.Code, -1)
				err := detectRun.Err
				h.AssertEq(t, err.Error(), `extension A outputs "requires" which is not allowed`)
			})

			it("errors if there is an alternate plan with requires", func() {
				mockEnv.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), someEnv), nil)

				toappfile("\n[[provides]]\n name = \"some-dep\"", "detect-plan-A-v1.toml")
				toappfile("\n[[or]]", "detect-plan-A-v1.toml")
				toappfile("\n[[or.requires]]\n name = \"other-dep\"", "detect-plan-A-v1.toml")

				detectRun := executor.Detect(descriptor, inputs, logger)

				h.AssertEq(t, detectRun.Code, -1)
				err := detectRun.Err
				h.AssertEq(t, err.Error(), `extension A outputs "requires" which is not allowed`)
			})

			when("/bin/detect is missing", func() {
				it.Before(func() {
					descriptorPath = filepath.Join("testdata", "extension", "by-id", "B", "v1", "extension.toml")
					var err error
					descriptor, err = buildpack.ReadExtDescriptor(filepath.Join("testdata", "extension", "by-id", "B", "v1", "extension.toml"))
					h.AssertNil(t, err)
					descriptor.WithAPI = api.Buildpack.Latest().String() // override
				})

				it("passes detection", func() {
					detectRun := executor.Detect(descriptor, inputs, logger)
					h.AssertEq(t, detectRun.Code, 0)

					t.Log("treats the extension root as a pre-populated output directory")
					h.AssertEq(t, detectRun.Provides, []buildpack.Provide{{Name: "some-dep"}})
				})

				when("plan is missing", func() {
					it.Before(func() {
						descriptorPath = filepath.Join("testdata", "extension", "by-id", "C", "v1", "extension.toml")
						var err error
						descriptor, err = buildpack.ReadExtDescriptor(descriptorPath)
						h.AssertNil(t, err)
						descriptor.WithAPI = api.Buildpack.Latest().String() // override
					})

					it("passes detection", func() {
						detectRun := executor.Detect(descriptor, inputs, logger)
						h.AssertEq(t, detectRun.Code, 0)

						t.Log("treats the extension root as a pre-populated output directory")
						var empty []buildpack.Provide
						h.AssertEq(t, detectRun.Provides, empty)
					})
				})
			})
		})
	})
}

func isUnset(actual string) bool {
	return actual == "unset"
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
