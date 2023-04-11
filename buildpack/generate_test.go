package buildpack_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/memory"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	llog "github.com/buildpacks/lifecycle/log"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

func TestGenerate(t *testing.T) {
	if runtime.GOOS != "windows" {
		spec.Run(t, "unit-generate", testGenerate, spec.Report(report.Terminal{}))
	}
}

func testGenerate(t *testing.T, when spec.G, it spec.S) {
	var tmpDir string

	it.Before(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "lifecycle")
		h.AssertNil(t, err)
	})

	it.After(func() {
		_ = os.RemoveAll(tmpDir)
	})

	when("#Generate", func() {
		var (
			mockCtrl   *gomock.Controller
			executor   buildpack.GenerateExecutor
			dirStore   string
			descriptor buildpack.ExtDescriptor

			// generate inputs
			inputs         buildpack.GenerateInputs
			appDir         string
			buildConfigDir string
			outputDir      string
			platformDir    string
			mockEnv        *testmock.MockBuildEnv
			stdout, stderr *bytes.Buffer

			logger     llog.Logger
			logHandler = memory.New()
		)

		it.Before(func() {
			mockCtrl = gomock.NewController(t)
			executor = &buildpack.DefaultGenerateExecutor{}

			// setup descriptor
			var err error
			dirStore, err = filepath.Abs(filepath.Join("testdata", "extension", "by-id"))
			h.AssertNil(t, err)
			descriptor = buildpack.ExtDescriptor{
				WithAPI: api.Buildpack.Latest().String(),
				Extension: buildpack.ExtInfo{
					BaseInfo: buildpack.BaseInfo{
						ID:       "A",
						Version:  "v1",
						Name:     "Extension A",
						ClearEnv: false,
						Homepage: "Extension A Homepage",
					},
				},
				WithRootDir: filepath.Join(dirStore, "A", "v1"),
			}

			// setup dirs
			outputDir = filepath.Join(tmpDir, "launch")
			appDir = filepath.Join(outputDir, "app")
			buildConfigDir = filepath.Join(tmpDir, "build-config")
			platformDir = filepath.Join(tmpDir, "platform")
			h.Mkdir(t, outputDir, appDir, filepath.Join(platformDir, "env"))

			// make inputs
			mockEnv = testmock.NewMockBuildEnv(mockCtrl)
			stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
			inputs = buildpack.GenerateInputs{
				AppDir:         appDir,
				BuildConfigDir: buildConfigDir,
				PlatformDir:    platformDir,
				Env:            mockEnv,
				OutputDir:      outputDir,
				Out:            stdout,
				Err:            stderr,
			}

			logger = &log.Logger{Handler: logHandler}
		})

		it.After(func() {
			mockCtrl.Finish()
		})

		when("env", func() {
			when("clear", func() {
				it.Before(func() {
					mockEnv.EXPECT().WithOverrides("", buildConfigDir).Return(append(os.Environ(), "TEST_ENV=cleared"), nil)

					descriptor.Extension.Version = "v1.clear"
					descriptor.WithRootDir = filepath.Join(dirStore, "A", "v1.clear")
					descriptor.Extension.ClearEnv = true
				})

				it("provides a clear env", func() {
					if _, err := executor.Generate(descriptor, inputs, logger); err != nil {
						t.Fatalf("Error: %s\n", err)
					}
					if s := cmp.Diff(h.Rdfile(t, filepath.Join(appDir, "build-info-A-v1.clear")),
						"TEST_ENV: cleared\n",
					); s != "" {
						t.Fatalf("Unexpected info:\n%s\n", s)
					}
				})

				it("sets CNB_ vars", func() {
					if _, err := executor.Generate(descriptor, inputs, logger); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}

					var actual string
					t.Log("sets CNB_EXTENSION_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-extension-dir-A-v1.clear"))
					h.AssertEq(t, actual, descriptor.WithRootDir)

					t.Log("sets CNB_PLATFORM_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-platform-dir-A-v1.clear"))
					h.AssertEq(t, actual, platformDir)

					t.Log("sets CNB_BP_PLAN_PATH")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-bp-plan-path-A-v1.clear"))
					if isUnset(actual) {
						t.Fatal("Expected CNB_BP_PLAN_PATH to be set")
					}

					t.Log("sets CNB_OUTPUT_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-output-dir-A-v1.clear"))
					h.AssertEq(t, actual, filepath.Join(outputDir, "A"))
					t.Log("does not set CNB_LAYERS_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-layers-dir-A-v1.clear"))
					h.AssertEq(t, isUnset(actual), true)
				})
			})

			when("full", func() {
				it.Before(func() {
					mockEnv.EXPECT().WithOverrides(platformDir, buildConfigDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
				})

				it("provides a full env", func() {
					if _, err := executor.Generate(descriptor, inputs, logger); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := cmp.Diff(h.Rdfile(t, filepath.Join(appDir, "build-info-A-v1")),
						"TEST_ENV: Av1\n",
					); s != "" {
						t.Fatalf("Unexpected info:\n%s\n", s)
					}
				})

				it("sets CNB_ vars", func() {
					if _, err := executor.Generate(descriptor, inputs, logger); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}

					var actual string
					t.Log("sets CNB_EXTENSION_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-extension-dir-A-v1"))
					h.AssertEq(t, actual, descriptor.WithRootDir)

					t.Log("sets CNB_PLATFORM_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-platform-dir-A-v1"))
					h.AssertEq(t, actual, platformDir)

					t.Log("sets CNB_BP_PLAN_PATH")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-bp-plan-path-A-v1"))
					if isUnset(actual) {
						t.Fatal("Expected CNB_BP_PLAN_PATH to be set")
					}

					t.Log("sets CNB_OUTPUT_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-output-dir-A-v1"))
					h.AssertEq(t, actual, filepath.Join(outputDir, "A"))
					t.Log("does not set CNB_LAYERS_DIR")
					actual = h.Rdfile(t, filepath.Join(appDir, "build-env-cnb-layers-dir-A-v1"))
					h.AssertEq(t, isUnset(actual), true)
				})

				it("loads env vars from <platform>/env", func() {
					h.Mkfile(t, "some-data",
						filepath.Join(platformDir, "env", "SOME_VAR"),
					)
					if _, err := executor.Generate(descriptor, inputs, logger); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					testExists(t,
						filepath.Join(appDir, "build-env-A-v1", "SOME_VAR"),
					)
				})
			})

			it("errors when <platform>/env cannot be loaded", func() {
				mockEnv.EXPECT().WithOverrides(platformDir, buildConfigDir).Return(nil, errors.New("some error"))
				if _, err := executor.Generate(descriptor, inputs, logger); err == nil {
					t.Fatal("Expected error.\n")
				} else if !strings.Contains(err.Error(), "some error") {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			when("any", func() {
				it.Before(func() {
					mockEnv.EXPECT().WithOverrides(platformDir, buildConfigDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil).AnyTimes()
				})

				it("errors when the provided buildpack plan is invalid", func() {
					inputs.Plan = buildpack.Plan{
						Entries: []buildpack.Require{
							{
								Metadata: map[string]interface{}{"a": map[int64]int64{1: 2}}, // map with non-string key type
							},
						},
					}
					if _, err := executor.Generate(descriptor, inputs, logger); err == nil {
						t.Fatal("Expected error.\n")
					} else if !strings.Contains(err.Error(), "toml") {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})

				it("connects stdout and stdin to the terminal", func() {
					if _, err := executor.Generate(descriptor, inputs, logger); err != nil {
						t.Fatalf("Unexpected error:\n%s\n", err)
					}
					if s := cmp.Diff(h.CleanEndings(stdout.String()), "build out: A@v1\n"); s != "" {
						t.Fatalf("Unexpected stdout:\n%s\n", s)
					}
					if s := cmp.Diff(h.CleanEndings(stderr.String()), "build err: A@v1\n"); s != "" {
						t.Fatalf("Unexpected stderr:\n%s\n", s)
					}
				})

				it("errors when the command fails", func() {
					if err := os.RemoveAll(platformDir); err != nil {
						t.Fatalf("Error: %s\n", err)
					}
					_, err := executor.Generate(descriptor, inputs, logger)
					if err, ok := err.(*buildpack.Error); !ok || err.Type != buildpack.ErrTypeBuildpack {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})

				when("build result", func() {
					when("dockerfiles", func() {
						when("run.Dockerfile", func() {
							it("is included", func() {
								h.Mkfile(t,
									"ARG base_image\n"+
										"FROM ${base_image}",
									filepath.Join(appDir, "run.Dockerfile-A-v1"),
								)

								br, err := executor.Generate(descriptor, inputs, logger)
								h.AssertNil(t, err)

								h.AssertEq(t, br.Dockerfiles[0].ExtensionID, "A")
								h.AssertEq(t, br.Dockerfiles[0].Kind, buildpack.DockerfileKindRun)
								h.AssertEq(t, br.Dockerfiles[0].Path, filepath.Join(outputDir, "A", "run.Dockerfile"))
								h.AssertEq(t, br.Dockerfiles[0].WithBase, "")
							})

							it("is validated", func() {
								h.Mkfile(t,
									"SOME-INVALID-CONTENT",
									filepath.Join(appDir, "run.Dockerfile-A-v1"),
								)
								_, err := executor.Generate(descriptor, inputs, logger)
								h.AssertError(t, err, "failed to parse run.Dockerfile for extension A: dockerfile parse error line 1: unknown instruction: SOME-INVALID-CONTENT")
							})

							when("switching the runtime base image", func() {
								it("image reference is included", func() {
									h.Mkfile(t,
										"FROM some-new-base-image",
										filepath.Join(appDir, "run.Dockerfile-A-v1"),
									)

									br, err := executor.Generate(descriptor, inputs, logger)
									h.AssertNil(t, err)

									h.AssertEq(t, br.Dockerfiles[0].ExtensionID, "A")
									h.AssertEq(t, br.Dockerfiles[0].Kind, buildpack.DockerfileKindRun)
									h.AssertEq(t, br.Dockerfiles[0].Path, filepath.Join(outputDir, "A", "run.Dockerfile"))
									h.AssertEq(t, br.Dockerfiles[0].WithBase, "some-new-base-image")
								})
							})
						})

						when("build.Dockerfile", func() {
							it("is included", func() {
								h.Mkfile(t,
									"ARG base_image\n"+
										"FROM ${base_image}",
									filepath.Join(appDir, "build.Dockerfile-A-v1"),
								)

								br, err := executor.Generate(descriptor, inputs, logger)
								h.AssertNil(t, err)

								h.AssertEq(t, br.Dockerfiles[0].ExtensionID, "A")
								h.AssertEq(t, br.Dockerfiles[0].Kind, buildpack.DockerfileKindBuild)
								h.AssertEq(t, br.Dockerfiles[0].Path, filepath.Join(outputDir, "A", "build.Dockerfile"))
							})

							it("is validated", func() {
								h.Mkfile(t,
									"SOME-INVALID-CONTENT",
									filepath.Join(appDir, "build.Dockerfile-A-v1"),
								)

								_, err := executor.Generate(descriptor, inputs, logger)
								h.AssertError(t, err, "failed to parse build.Dockerfile for extension A: dockerfile parse error line 1: unknown instruction: SOME-INVALID-CONTENT")
							})
						})
					})

					when("met requires", func() {
						it("are derived from input plan.toml", func() {
							inputs.Plan = buildpack.Plan{
								Entries: []buildpack.Require{
									{Name: "some-dep"},
									{Name: "some-other-dep"},
								},
							}
							h.Mkdir(t, filepath.Join(appDir, "generate"))
							h.Mkfile(t,
								"[[unmet]]\n"+
									`name = "some-other-dep"`+"\n",
								filepath.Join(appDir, "generate", "build-A-v1.toml"),
							)

							br, err := executor.Generate(descriptor, inputs, logger)
							h.AssertNil(t, err)

							h.AssertEq(t, br.MetRequires, []string{"some-dep", "some-other-dep"})
						})
					})

					when("/bin/build is missing", func() {
						it.Before(func() {
							descriptor.Extension.ID = "B"
							descriptor.WithRootDir = filepath.Join(dirStore, "B", "v1")
						})

						it("treats the extension root as a pre-populated output directory", func() {
							inputs.Plan = buildpack.Plan{
								Entries: []buildpack.Require{
									{Name: "some-dep"},
									{Name: "some-other-dep"},
								},
							}

							br, err := executor.Generate(descriptor, inputs, logger)
							h.AssertNil(t, err)

							t.Log("processes build.toml")
							h.AssertEq(t, br.MetRequires, []string{"some-dep", "some-other-dep"})
							t.Log("processes run.Dockerfile")
							h.AssertEq(t, br.Dockerfiles[0].ExtensionID, "B")
							h.AssertEq(t, br.Dockerfiles[0].Kind, buildpack.DockerfileKindRun)
							h.AssertEq(t, br.Dockerfiles[0].Path, filepath.Join(descriptor.WithRootDir, "generate", "run.Dockerfile"))
						})
					})
				})
			})
		})
	})
}
