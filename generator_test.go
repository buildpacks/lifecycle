package lifecycle_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/apex/log/handlers/memory"
	"github.com/golang/mock/gomock"
	"github.com/pkg/errors"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	llog "github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

func TestGenerator(t *testing.T) {
	if runtime.GOOS != "windows" {
		spec.Run(t, "unit-new-generator", testGeneratorFactory, spec.Report(report.Terminal{}))
		spec.Run(t, "unit-generator", testGenerator, spec.Report(report.Terminal{}))
	}
}

func testGeneratorFactory(t *testing.T, when spec.G, it spec.S) {
	when("#NewGenerator", func() {
		var (
			generatorFactory  *lifecycle.GeneratorFactory
			fakeAPIVerifier   *testmock.MockBuildpackAPIVerifier
			fakeConfigHandler *testmock.MockConfigHandler
			fakeDirStore      *testmock.MockDirStore
			logger            *log.Logger
			mockController    *gomock.Controller
			stdout, stderr    *bytes.Buffer
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			fakeAPIVerifier = testmock.NewMockBuildpackAPIVerifier(mockController)
			fakeConfigHandler = testmock.NewMockConfigHandler(mockController)
			fakeDirStore = testmock.NewMockDirStore(mockController)
			logger = &log.Logger{Handler: &discard.Handler{}}

			generatorFactory = lifecycle.NewGeneratorFactory(
				fakeAPIVerifier,
				fakeConfigHandler,
				fakeDirStore,
			)
		})

		it.After(func() {
			mockController.Finish()
		})

		it("configures the generator", func() {
			fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindExtension, "A@v1", "0.9", logger)
			fakeConfigHandler.EXPECT().ReadAnalyzed("some-analyzed-path").Return(platform.AnalyzedMetadata{RunImage: &platform.RunImage{Reference: "some-run-image-ref"}}, nil)
			fakeConfigHandler.EXPECT().ReadRun("some-run-path", logger).Return(platform.RunMetadata{Images: []platform.RunImageMetadata{{Image: "some-run-image"}}}, nil)

			providedPlan := platform.BuildPlan{Entries: []platform.BuildPlanEntry{
				{
					Providers: []buildpack.GroupElement{
						{ID: "A", Version: "v1", API: "0.9", Extension: true},
					},
					Requires: []buildpack.Require{
						{Name: "some-dep"},
					},
				},
			}}
			generator, err := generatorFactory.NewGenerator(
				"some-analyzed-path",
				"some-app-dir",
				"some-build-config-dir",
				[]buildpack.GroupElement{
					{ID: "A", Version: "v1", API: "0.9"},
				},
				"some-output-dir",
				providedPlan,
				"some-platform-dir",
				"some-run-path",
				stdout, stderr,
				logger,
			)
			h.AssertNil(t, err)

			h.AssertEq(t, generator.AnalyzedMD, platform.AnalyzedMetadata{RunImage: &platform.RunImage{Reference: "some-run-image-ref"}})
			h.AssertEq(t, generator.AppDir, "some-app-dir")
			h.AssertNotNil(t, generator.DirStore)
			h.AssertEq(t, generator.Extensions, []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.9"},
			})
			h.AssertEq(t, generator.GeneratedDir, "some-output-dir")
			h.AssertEq(t, generator.Logger, logger)
			h.AssertEq(t, generator.Plan, providedPlan)
			h.AssertEq(t, generator.PlatformDir, "some-platform-dir")
			h.AssertEq(t, generator.RunMetadata, platform.RunMetadata{Images: []platform.RunImageMetadata{{Image: "some-run-image"}}})
			h.AssertEq(t, generator.Out, stdout)
			h.AssertEq(t, generator.Err, stderr)
		})
	})
}

func testGenerator(t *testing.T, when spec.G, it spec.S) {
	var (
		mockCtrl       *gomock.Controller
		generator      *lifecycle.Generator
		tmpDir         string
		appDir         string
		generatedDir   string
		platformDir    string
		dirStore       *testmock.MockDirStore
		executor       *testmock.MockGenerateExecutor
		logHandler     = memory.New()
		stdout, stderr *bytes.Buffer
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		dirStore = testmock.NewMockDirStore(mockCtrl)
		executor = testmock.NewMockGenerateExecutor(mockCtrl)

		var err error
		tmpDir, err = os.MkdirTemp("", "lifecycle")
		h.AssertNil(t, err)
		generatedDir = filepath.Join(tmpDir, "output")
		appDir = filepath.Join(generatedDir, "app")
		platformDir = filepath.Join(tmpDir, "platform")
		h.Mkdir(t, generatedDir, appDir, filepath.Join(platformDir, "env"))
		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}

		generator = &lifecycle.Generator{
			AnalyzedMD: platform.AnalyzedMetadata{},
			AppDir:     appDir,
			DirStore:   dirStore,
			Executor:   executor,
			Extensions: []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: api.Buildpack.Latest().String(), Homepage: "A Homepage"},
				{ID: "ext/B", Version: "v2", API: api.Buildpack.Latest().String()},
			},
			Logger:       &log.Logger{Handler: logHandler},
			GeneratedDir: generatedDir,
			Plan:         platform.BuildPlan{},
			PlatformDir:  platformDir,
			Err:          stderr,
			Out:          stdout,
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
		mockCtrl.Finish()
	})

	when(".Generate", func() {
		extA := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
		extB := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "ext/B", Version: "v1"}}}
		extC := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "C", Version: "v1"}}}

		it("provides a subset of the build plan to each extension", func() {
			generator.Plan = platform.BuildPlan{
				Entries: []platform.BuildPlanEntry{
					{
						Providers: []buildpack.GroupElement{
							{ID: "A", Version: "v1"}, // not provided to any extension because Extension is false
						},
						Requires: []buildpack.Require{
							{Name: "buildpack-dep", Version: "v1"},
						},
					},
					{
						Providers: []buildpack.GroupElement{
							{ID: "A", Version: "v1", Extension: true},
							{ID: "ext/B", Version: "v2", Extension: true},
						},
						Requires: []buildpack.Require{
							{Name: "some-dep", Version: "v1"}, // not provided to B because it is met
						},
					},
					{
						Providers: []buildpack.GroupElement{
							{ID: "A", Version: "v1", Extension: true},
							{ID: "ext/B", Version: "v2", Extension: true},
						},
						Requires: []buildpack.Require{
							{Name: "some-unmet-dep", Version: "v2"}, // provided to ext/B because it is unmet
						},
					},
					{
						Providers: []buildpack.GroupElement{
							{ID: "ext/B", Version: "v2", Extension: true},
						},
						Requires: []buildpack.Require{
							{Name: "other-dep", Version: "v4"}, // only provided to ext/B
						},
					},
				},
			}
			expectedInputs := buildpack.GenerateInputs{
				AppDir:      appDir,
				PlatformDir: platformDir,
				// OutputDir is ephemeral directory
			}

			dirStore.EXPECT().LookupExt("A", "v1").Return(&extA, nil)
			expectedAInputs := expectedInputs
			expectedAInputs.Plan = buildpack.Plan{Entries: []buildpack.Require{
				{Name: "some-dep", Version: "v1"},
				{Name: "some-unmet-dep", Version: "v2"},
			}}
			executor.EXPECT().Generate(extA, gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ buildpack.ExtDescriptor, inputs buildpack.GenerateInputs, _ llog.Logger) (buildpack.GenerateOutputs, error) {
					h.AssertEq(t, inputs.Plan, expectedAInputs.Plan)
					h.AssertEq(t, inputs.AppDir, expectedAInputs.AppDir)
					h.AssertEq(t, inputs.PlatformDir, expectedAInputs.PlatformDir)
					h.AssertEq(t, inputs.BuildConfigDir, generator.BuildConfigDir)
					return buildpack.GenerateOutputs{MetRequires: []string{"some-dep"}}, nil
				})

			dirStore.EXPECT().LookupExt("ext/B", "v2").Return(&extB, nil)
			expectedBInputs := expectedInputs
			expectedBInputs.Plan = buildpack.Plan{Entries: []buildpack.Require{
				{Name: "some-unmet-dep", Version: "v2"},
				{Name: "other-dep", Version: "v4"},
			}}
			executor.EXPECT().Generate(extB, gomock.Any(), gomock.Any()).Do(
				func(_ buildpack.ExtDescriptor, inputs buildpack.GenerateInputs, _ llog.Logger) (buildpack.GenerateOutputs, error) {
					h.AssertEq(t, inputs.Plan, expectedBInputs.Plan)
					h.AssertEq(t, inputs.AppDir, expectedBInputs.AppDir)
					h.AssertEq(t, inputs.PlatformDir, expectedBInputs.PlatformDir)
					h.AssertEq(t, inputs.BuildConfigDir, generator.BuildConfigDir)
					return buildpack.GenerateOutputs{}, nil
				})

			_, err := generator.Generate()
			h.AssertNil(t, err)
		})

		it("copies Dockerfiles and extend-config.toml files to the correct locations", func() {
			// mock generate for extension A
			dirStore.EXPECT().LookupExt("A", "v1").Return(&extA, nil)
			// extension A has a build.Dockerfile and an extend-config.toml
			h.Mkdir(t, filepath.Join(tmpDir, "A"))
			buildDockerfilePathA := filepath.Join(tmpDir, "A", "build.Dockerfile")
			h.Mkfile(t, "some-build.Dockerfile-content-A", buildDockerfilePathA)
			extendConfigPathA := filepath.Join(tmpDir, "A", "extend-config.toml")
			h.Mkfile(t, "some-extend-config.toml-content-A", extendConfigPathA)
			executor.EXPECT().Generate(extA, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
				Dockerfiles: []buildpack.DockerfileInfo{
					{
						ExtensionID: "A",
						Kind:        "build",
						Path:        buildDockerfilePathA,
					},
				},
			}, nil)

			// mock generate for extension B
			dirStore.EXPECT().LookupExt("ext/B", "v2").Return(&extB, nil)
			// extension B has a run.Dockerfile
			h.Mkdir(t, filepath.Join(tmpDir, "B"))
			runDockerfilePathB := filepath.Join(tmpDir, "B", "build.Dockerfile")
			h.Mkfile(t, "some-run.Dockerfile-content-B", runDockerfilePathB)
			executor.EXPECT().Generate(extB, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
				Dockerfiles: []buildpack.DockerfileInfo{
					{
						ExtensionID: "B",
						Kind:        "run",
						Path:        runDockerfilePathB,
					},
				},
			}, nil)

			// mock generate for extension C
			dirStore.EXPECT().LookupExt("C", "v1").Return(&extC, nil)
			// extension C has a build.Dockerfile, run.Dockerfile, and extend-config.toml
			h.Mkdir(t, filepath.Join(tmpDir, "C"))
			buildDockerfilePathC := filepath.Join(tmpDir, "C", "build.Dockerfile")
			h.Mkfile(t, "some-build.Dockerfile-content-C", buildDockerfilePathC)
			runDockerfilePathC := filepath.Join(tmpDir, "C", "run.Dockerfile")
			h.Mkfile(t, "some-run.Dockerfile-content-C", runDockerfilePathC)
			extendConfigPathC := filepath.Join(tmpDir, "C", "extend-config.toml")
			h.Mkfile(t, "some-extend-config.toml-content-C", extendConfigPathC)
			executor.EXPECT().Generate(extC, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
				Dockerfiles: []buildpack.DockerfileInfo{
					{
						ExtensionID: "C",
						Kind:        "build",
						Path:        buildDockerfilePathC,
					},
					{
						ExtensionID: "C",
						Kind:        "run",
						Path:        runDockerfilePathC,
					},
				},
			}, nil)

			// add extension C to the group
			generator.Extensions = append(generator.Extensions, buildpack.GroupElement{ID: "C", Version: "v1", API: api.Buildpack.Latest().String()})
			// do generate
			_, err := generator.Generate()
			h.AssertNil(t, err)

			t.Log("copies Dockerfiles")
			contents := h.MustReadFile(t, filepath.Join(generatedDir, "build", "A", "Dockerfile"))
			h.AssertEq(t, string(contents), "some-build.Dockerfile-content-A")
			contents = h.MustReadFile(t, filepath.Join(generatedDir, "run", "B", "Dockerfile"))
			h.AssertEq(t, string(contents), "some-run.Dockerfile-content-B")
			contents = h.MustReadFile(t, filepath.Join(generatedDir, "build", "C", "Dockerfile"))
			h.AssertEq(t, string(contents), "some-build.Dockerfile-content-C")
			contents = h.MustReadFile(t, filepath.Join(generatedDir, "run", "C", "Dockerfile"))
			h.AssertEq(t, string(contents), "some-run.Dockerfile-content-C")

			t.Log("copies extend-config.toml files if exist")
			contents = h.MustReadFile(t, filepath.Join(generatedDir, "build", "A", "extend-config.toml"))
			h.AssertEq(t, string(contents), "some-extend-config.toml-content-A")
			contents = h.MustReadFile(t, filepath.Join(generatedDir, "build", "C", "extend-config.toml"))
			h.AssertEq(t, string(contents), "some-extend-config.toml-content-C")
			contents = h.MustReadFile(t, filepath.Join(generatedDir, "run", "C", "extend-config.toml"))
			h.AssertEq(t, string(contents), "some-extend-config.toml-content-C")

			t.Log("does not pollute the output directory")
			h.AssertPathDoesNotExist(t, filepath.Join(generatedDir, "A", "run.Dockerfile"))
			h.AssertPathDoesNotExist(t, filepath.Join(generatedDir, "B", "build.Dockerfile"))
			h.AssertPathDoesNotExist(t, filepath.Join(generatedDir, "C", "run.Dockerfile"))
			h.AssertPathDoesNotExist(t, filepath.Join(generatedDir, "C", "build.Dockerfile"))
		})

		when("determining the correct run image", func() {
			var runDockerfilePathA, runDockerfilePathB string

			it.Before(func() {
				runDockerfilePathA = filepath.Join(tmpDir, "run.Dockerfile.A")
				h.Mkfile(t, "some-dockerfile-content-A", runDockerfilePathA)
				runDockerfilePathB = filepath.Join(tmpDir, "run.Dockerfile.B")
				h.Mkfile(t, "some-dockerfile-content-B", runDockerfilePathB)
			})

			when("all run.Dockerfiles declare `FROM ${base_image}`", func() {
				it("returns the original run image in the result", func() {
					generator.AnalyzedMD = platform.AnalyzedMetadata{
						RunImage: &platform.RunImage{
							Reference: "some-existing-run-image",
						},
					}

					// mock generate for extension A
					dirStore.EXPECT().LookupExt("A", "v1").Return(&extA, nil)
					executor.EXPECT().Generate(extA, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
						Dockerfiles: []buildpack.DockerfileInfo{
							{
								ExtensionID: "A",
								Kind:        "run",
								Path:        runDockerfilePathA,
								Base:        "",
							},
						},
					}, nil)

					// mock generate for extension B
					dirStore.EXPECT().LookupExt("ext/B", "v2").Return(&extB, nil)
					executor.EXPECT().Generate(extB, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{}, nil)

					// do generate
					result, err := generator.Generate()
					h.AssertNil(t, err)

					h.AssertEq(t, result.AnalyzedMD.RunImage.Reference, "some-existing-run-image")
					t.Log("sets extend to true in the result")
					h.AssertEq(t, result.AnalyzedMD.RunImage.Extend, true)

					t.Log("copies Dockerfiles to the correct locations")
					aContents := h.MustReadFile(t, filepath.Join(generatedDir, "run", "A", "Dockerfile"))
					h.AssertEq(t, string(aContents), `some-dockerfile-content-A`)
				})
			})

			when("run.Dockerfiles use FROM to switch the run image", func() {
				it("returns the last image referenced in the `FROM` statement of the last run.Dockerfile not to declare `FROM ${base_image}`", func() {
					generator.AnalyzedMD = platform.AnalyzedMetadata{
						RunImage: &platform.RunImage{
							Reference: "some-existing-run-image",
						},
					}

					// mock generate for extension A
					dirStore.EXPECT().LookupExt("A", "v1").Return(&extA, nil)
					executor.EXPECT().Generate(extA, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
						Dockerfiles: []buildpack.DockerfileInfo{
							{
								ExtensionID: "A",
								Kind:        "run",
								Path:        runDockerfilePathA,
								Base:        "some-new-base-image",
							},
						},
					}, nil)

					// mock generate for extension B
					dirStore.EXPECT().LookupExt("ext/B", "v2").Return(&extB, nil)
					executor.EXPECT().Generate(extB, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
						Dockerfiles: []buildpack.DockerfileInfo{
							{
								ExtensionID: "B",
								Kind:        "run",
								Path:        runDockerfilePathB,
								Base:        "",
							},
						},
					}, nil)

					// do generate
					result, err := generator.Generate()
					h.AssertNil(t, err)

					h.AssertEq(t, result.AnalyzedMD.RunImage.Reference, "some-new-base-image")
					t.Log("sets extend to true in the result")
					h.AssertEq(t, result.AnalyzedMD.RunImage.Extend, true)

					t.Log("copies Dockerfiles to the correct locations")
					aContents := h.MustReadFile(t, filepath.Join(generatedDir, "run", "A", "Dockerfile"))
					h.AssertEq(t, string(aContents), `some-dockerfile-content-A`)
					BContents := h.MustReadFile(t, filepath.Join(generatedDir, "run", "B", "Dockerfile"))
					h.AssertEq(t, string(BContents), `some-dockerfile-content-B`)
				})

				when("no more run.Dockerfiles follow", func() {
					it("sets extend to false in the result", func() {
						// mock generate for extension A
						dirStore.EXPECT().LookupExt("A", "v1").Return(&extA, nil)
						executor.EXPECT().Generate(extA, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
							Dockerfiles: []buildpack.DockerfileInfo{
								{
									ExtensionID: "A",
									Kind:        "run",
									Path:        runDockerfilePathA,
									Base:        "some-new-base-image",
								},
							},
						}, nil)

						// mock generate for extension B
						dirStore.EXPECT().LookupExt("ext/B", "v2").Return(&extB, nil)
						executor.EXPECT().Generate(extB, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
							Dockerfiles: []buildpack.DockerfileInfo{
								{
									ExtensionID: "B",
									Kind:        "run",
									Path:        runDockerfilePathB,
									Base:        "some-other-base-image",
								},
							},
						}, nil)

						// do generate
						result, err := generator.Generate()
						h.AssertNil(t, err)

						h.AssertEq(t, result.AnalyzedMD.RunImage.Reference, "some-other-base-image")
						h.AssertEq(t, result.AnalyzedMD.RunImage.Extend, false)

						t.Log("copies Dockerfiles to the correct locations")
						t.Log("renames earlier run.Dockerfiles to Dockerfile.ignore in the output directory")
						aContents := h.MustReadFile(t, filepath.Join(generatedDir, "run", "A", "Dockerfile.ignore"))
						h.AssertEq(t, string(aContents), `some-dockerfile-content-A`)
						BContents := h.MustReadFile(t, filepath.Join(generatedDir, "run", "B", "Dockerfile"))
						h.AssertEq(t, string(BContents), `some-dockerfile-content-B`)
					})
				})

				when("run metadata provided", func() {
					it.Before(func() {
						generator.RunMetadata = platform.RunMetadata{
							Images: []platform.RunImageMetadata{
								{Image: "some-run-image"},
							},
						}
					})

					when("containing new run image", func() {
						it("succeeds", func() {
							// mock generate for extension A
							dirStore.EXPECT().LookupExt("A", "v1").Return(&extA, nil)
							executor.EXPECT().Generate(extA, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
								Dockerfiles: []buildpack.DockerfileInfo{
									{
										ExtensionID: "A",
										Kind:        "run",
										Path:        runDockerfilePathA,
										Base:        "some-run-image",
									},
								},
							}, nil)

							// mock generate for extension B
							dirStore.EXPECT().LookupExt("ext/B", "v2").Return(&extB, nil)
							executor.EXPECT().Generate(extB, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{}, nil)

							// do generate
							result, err := generator.Generate()
							h.AssertNil(t, err)

							h.AssertEq(t, result.AnalyzedMD.RunImage.Reference, "some-run-image")
						})
					})

					when("not containing new run image", func() {
						it("errors", func() {
							// mock generate for extension A
							dirStore.EXPECT().LookupExt("A", "v1").Return(&extA, nil)
							executor.EXPECT().Generate(extA, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
								Dockerfiles: []buildpack.DockerfileInfo{
									{
										ExtensionID: "A",
										Kind:        "run",
										Path:        runDockerfilePathA,
										Base:        "some-other-run-image",
									},
								},
							}, nil)

							// mock generate for extension B
							dirStore.EXPECT().LookupExt("ext/B", "v2").Return(&extB, nil)
							executor.EXPECT().Generate(extB, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{}, nil)

							// do generate
							_, err := generator.Generate()
							h.AssertError(t, err, "new runtime base image 'some-other-run-image' not found in run metadata")
						})
					})
				})
			})
		})

		when("extension generate failed", func() {
			when("first extension fails", func() {
				it("errors", func() {
					extA := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
					dirStore.EXPECT().LookupExt("A", "v1").Return(&extA, nil)
					executor.EXPECT().Generate(extA, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{}, errors.New("some error"))

					if _, err := generator.Generate(); err == nil {
						t.Fatal("Expected error.\n")
					} else if !strings.Contains(err.Error(), "some error") {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})
			})

			when("later extension fails", func() {
				it("errors", func() {
					extA := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
					dirStore.EXPECT().LookupExt("A", "v1").Return(&extA, nil)
					executor.EXPECT().Generate(extA, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{}, nil)
					extB := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "ext/B", Version: "v1"}}}
					dirStore.EXPECT().LookupExt("ext/B", "v2").Return(&extB, nil)
					executor.EXPECT().Generate(extB, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{}, errors.New("some error"))

					if _, err := generator.Generate(); err == nil {
						t.Fatal("Expected error.\n")
					} else if !strings.Contains(err.Error(), "some error") {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})
			})
		})
	})
}
