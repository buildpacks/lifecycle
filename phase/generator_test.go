package phase_test

import (
	"bytes"
	"fmt"
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

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	llog "github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/phase/testmock"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
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
			generatorFactory  *phase.HermeticFactory
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

			generatorFactory = phase.NewHermeticFactory(
				api.Platform.Latest(),
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
			fakeConfigHandler.EXPECT().ReadAnalyzed("some-analyzed-path", logger).Return(files.Analyzed{RunImage: &files.RunImage{Reference: "some-run-image-ref"}}, nil)
			fakeConfigHandler.EXPECT().ReadRun("some-run-path", logger).Return(files.Run{Images: []files.RunImageForExport{{Image: "some-run-image"}}}, nil)
			providedExtensions := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.9", Extension: true},
			}
			fakeConfigHandler.EXPECT().ReadGroup("some-group-path").Return(buildpack.Group{GroupExtensions: providedExtensions}, nil)
			providedPlan := files.Plan{Entries: []files.BuildPlanEntry{
				{
					Providers: []buildpack.GroupElement{
						{ID: "A", Version: "v1", API: "0.9", Extension: true},
					},
					Requires: []buildpack.Require{
						{Name: "some-dep"},
					},
				},
			}}
			fakeConfigHandler.EXPECT().ReadPlan("some-plan-path").Return(providedPlan, nil)

			generator, err := generatorFactory.NewGenerator(platform.LifecycleInputs{
				AnalyzedPath:   "some-analyzed-path",
				AppDir:         "some-app-dir",
				BuildConfigDir: "some-build-config-dir",
				GroupPath:      "some-group-path",
				GeneratedDir:   "some-output-dir",
				PlanPath:       "some-plan-path",
				PlatformDir:    "some-platform-dir",
				RunPath:        "some-run-path",
			}, stdout, stderr, logger,
			)
			h.AssertNil(t, err)

			h.AssertEq(t, generator.AnalyzedMD, files.Analyzed{RunImage: &files.RunImage{Reference: "some-run-image-ref"}})
			h.AssertEq(t, generator.AppDir, "some-app-dir")
			h.AssertNotNil(t, generator.DirStore)
			h.AssertEq(t, generator.Extensions, []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.9", Extension: true},
			})
			h.AssertEq(t, generator.GeneratedDir, "some-output-dir")
			h.AssertEq(t, generator.Logger, logger)
			h.AssertEq(t, generator.Plan, providedPlan)
			h.AssertEq(t, generator.PlatformDir, "some-platform-dir")
			h.AssertEq(t, generator.RunMetadata, files.Run{Images: []files.RunImageForExport{{Image: "some-run-image"}}})
			h.AssertEq(t, generator.Out, stdout)
			h.AssertEq(t, generator.Err, stderr)
		})
	})
}

func testGenerator(t *testing.T, when spec.G, it spec.S) {
	var (
		mockCtrl       *gomock.Controller
		generator      *phase.Generator
		tmpDir         string
		appDir         string
		generatedDir   string
		platformDir    string
		dirStore       *testmock.MockDirStore
		executor       *testmock.MockGenerateExecutor
		logHandler     = memory.New()
		stdout, stderr *bytes.Buffer
	)

	// We need to create the temp directory before `it.Before` blocks because we want the variable to be populated
	// when we set up the table tests.
	var tmpErr error
	tmpDir, tmpErr = os.MkdirTemp("", "lifecycle")
	generatedDir = filepath.Join(tmpDir, "output")
	appDir = filepath.Join(generatedDir, "app")
	platformDir = filepath.Join(tmpDir, "platform")

	it.Before(func() {
		h.AssertNil(t, tmpErr)

		mockCtrl = gomock.NewController(t)
		dirStore = testmock.NewMockDirStore(mockCtrl)
		executor = testmock.NewMockGenerateExecutor(mockCtrl)

		h.Mkdir(t, generatedDir, appDir, filepath.Join(platformDir, "env"))
		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}

		generator = &phase.Generator{
			AnalyzedMD: files.Analyzed{},
			AppDir:     appDir,
			DirStore:   dirStore,
			Executor:   executor,
			Extensions: []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: api.Buildpack.Latest().String(), Homepage: "A Homepage"},
				{ID: "ext/B", Version: "v2", API: api.Buildpack.Latest().String()},
			},
			Logger:       &log.Logger{Handler: logHandler},
			GeneratedDir: generatedDir,
			Plan:         files.Plan{},
			PlatformDir:  platformDir,
			Err:          stderr,
			Out:          stdout,
		}
	})

	it.After(func() {
		_ = os.RemoveAll(tmpDir)
		mockCtrl.Finish()
	})

	when(".Generate", func() {
		extA := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
		extB := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "ext/B", Version: "v1"}}}
		extC := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "C", Version: "v1"}}}

		for _, platformAPI := range []*api.Version{
			api.MustParse("0.12"),
			api.MustParse("0.13"),
		} {
			platformAPI := platformAPI

			when(fmt.Sprintf("using the platform API %s", platformAPI), func() {
				it.Before(func() {
					generator.PlatformAPI = platformAPI
				})

				it("provides a subset of the build plan to each extension", func() {
					generator.Plan = files.Plan{
						Entries: []files.BuildPlanEntry{
							{
								Providers: []buildpack.GroupElement{
									{ID: "some-buildpack", Version: "v1"}, // not provided to any extension because Extension is false
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

				it("passes through CNB_TARGET environment variables", func() {
					generator.AnalyzedMD = files.Analyzed{
						RunImage: &files.RunImage{
							TargetMetadata: &files.TargetMetadata{
								OS:   "linux",
								Arch: "amd64",
							},
						},
					}
					// mock generate for extensions  - these are tested elsewhere, so we just need to return anything...
					dirStore.EXPECT().LookupExt(gomock.Any(), gomock.Any()).Return(&extA, nil)
					dirStore.EXPECT().LookupExt(gomock.Any(), gomock.Any()).Return(&extB, nil)
					buildDockerfilePathA := filepath.Join(tmpDir, "dummy.Dockerfile")
					h.Mkfile(t, "some-build.Dockerfile-content-A", buildDockerfilePathA)
					executor.EXPECT().Generate(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
						func(d buildpack.ExtDescriptor, inputs buildpack.GenerateInputs, _ *log.Logger) (buildpack.GenerateOutputs, error) {
							h.AssertContains(t, inputs.TargetEnv,
								"CNB_TARGET_ARCH=amd64",
								"CNB_TARGET_OS=linux",
							)
							return buildpack.GenerateOutputs{Dockerfiles: []buildpack.DockerfileInfo{{ExtensionID: d.Extension.ID,
								Kind: "build", Path: buildDockerfilePathA}}}, nil
						})
					executor.EXPECT().Generate(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
						func(d buildpack.ExtDescriptor, inputs buildpack.GenerateInputs, _ *log.Logger) (buildpack.GenerateOutputs, error) {
							h.AssertContains(t, inputs.TargetEnv,
								"CNB_TARGET_ARCH=amd64",
								"CNB_TARGET_OS=linux",
							)
							return buildpack.GenerateOutputs{Dockerfiles: []buildpack.DockerfileInfo{{ExtensionID: d.Extension.ID,
								Kind: "build", Path: buildDockerfilePathA}}}, nil
						})
					_, err := generator.Generate()
					h.AssertNil(t, err)
				})

				if platformAPI.LessThan("0.13") { // Platform API >= 0.13 does no longer require Dockerfile copying
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
						runDockerfilePathB := filepath.Join(tmpDir, "B", "run.Dockerfile")
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

						t.Log("copies extend-config.toml files if they exist")
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
				}

				when("returning run image metadata", func() {
					var runDockerfilePathA, runDockerfilePathB string
					if platformAPI.AtLeast("0.13") {
						runDockerfilePathA = filepath.Join(generatedDir, "A", "run.Dockerfile.A")
						runDockerfilePathB = filepath.Join(generatedDir, "B", "run.Dockerfile.B")
					} else {
						runDockerfilePathA = filepath.Join(tmpDir, "run.Dockerfile.A")
						runDockerfilePathB = filepath.Join(tmpDir, "run.Dockerfile.B")
					}

					it.Before(func() {
						h.Mkdir(t, filepath.Dir(runDockerfilePathA), filepath.Dir(runDockerfilePathB))
						h.Mkfile(t, "some-dockerfile-content-A", runDockerfilePathA)
						h.Mkfile(t, "some-dockerfile-content-B", runDockerfilePathB)

						generator.AnalyzedMD = files.Analyzed{
							RunImage: &files.RunImage{
								Reference: "some-existing-run-image@sha256:s0m3d1g3st",
								Image:     "some-existing-run-image",
							},
						}
					})

					type testCase struct {
						before                    func()
						descCondition             string
						descResult                string
						aDockerfiles              []buildpack.DockerfileInfo
						bDockerfiles              []buildpack.DockerfileInfo
						expectedRunImageImage     string
						expectedRunImageReference string
						expectedRunImageExtend    bool
						expectedErr               string
						assertAfter               func()
					}
					for _, tc := range []testCase{
						{
							descCondition: "all run.Dockerfiles declare `FROM ${base_image}`",
							descResult:    "returns the original run image in the result",
							aDockerfiles: []buildpack.DockerfileInfo{{
								ExtensionID: "A",
								Kind:        "run",
								Path:        runDockerfilePathA,
								WithBase:    "",
								Extend:      true,
							}},
							bDockerfiles: []buildpack.DockerfileInfo{{
								ExtensionID: "B",
								Kind:        "run",
								Path:        runDockerfilePathB,
								WithBase:    "",
								Extend:      true,
							}},
							expectedRunImageImage:     "some-existing-run-image",
							expectedRunImageReference: "some-existing-run-image@sha256:s0m3d1g3st",
							expectedRunImageExtend:    true,
						},
						{
							descCondition: "a run.Dockerfile declares a new base image and run.Dockerfiles follow",
							descResult:    "returns the last image referenced in the `FROM` statement of the last run.Dockerfile not to declare `FROM ${base_image}`, and sets extend to true",
							aDockerfiles: []buildpack.DockerfileInfo{
								{
									ExtensionID: "A",
									Kind:        "run",
									Path:        runDockerfilePathA,
									WithBase:    "some-new-run-image",
									Extend:      false,
								},
							},
							bDockerfiles: []buildpack.DockerfileInfo{
								{
									ExtensionID: "B",
									Kind:        "run",
									Path:        runDockerfilePathB,
									WithBase:    "",
									Extend:      true,
								},
							},
							expectedRunImageImage:     "some-new-run-image",
							expectedRunImageReference: "some-new-run-image",
							expectedRunImageExtend:    true,
						},
						{
							descCondition: "a run.Dockerfile declares a new base image (only) and no run.Dockerfiles follow",
							descResult:    "returns the referenced base image, and sets extend to false",
							aDockerfiles: []buildpack.DockerfileInfo{
								{
									ExtensionID: "A",
									Kind:        "run",
									Path:        runDockerfilePathA,
									WithBase:    "some-new-run-image",
									Extend:      true,
								},
							},
							bDockerfiles: []buildpack.DockerfileInfo{
								{
									ExtensionID: "B",
									Kind:        "run",
									Path:        runDockerfilePathB,
									WithBase:    "some-other-base-image",
									Extend:      false,
								},
							},
							expectedRunImageImage:     "some-other-base-image",
							expectedRunImageReference: "some-other-base-image",
							expectedRunImageExtend:    false,
							assertAfter: func() {
								var runDockerfileDestinationPathA, runDockerfileDestinationPathB string
								if platformAPI.AtLeast("0.13") {
									runDockerfileDestinationPathA = runDockerfilePathA + ".ignore"
									runDockerfileDestinationPathB = runDockerfilePathB
								} else {
									runDockerfileDestinationPathA = filepath.Join(generatedDir, "run", "A", "Dockerfile.ignore")
									runDockerfileDestinationPathB = filepath.Join(generatedDir, "run", "B", "Dockerfile")
									t.Log("copies Dockerfiles to the correct locations")
								}
								t.Log("renames earlier run.Dockerfiles to Dockerfile.ignore in the output directory")
								aContents := h.MustReadFile(t, runDockerfileDestinationPathA)
								h.AssertEq(t, string(aContents), `some-dockerfile-content-A`)
								bContents := h.MustReadFile(t, runDockerfileDestinationPathB)
								h.AssertEq(t, string(bContents), `some-dockerfile-content-B`)
							},
						},
						{
							descCondition: "a run.Dockerfile declares a new base image and also extends",
							descResult:    "returns the referenced base image, and sets extend to true",
							aDockerfiles: []buildpack.DockerfileInfo{
								{
									ExtensionID: "A",
									Kind:        "run",
									Path:        runDockerfilePathA,
									WithBase:    "some-new-run-image",
									Extend:      true,
								},
							},
							bDockerfiles:              []buildpack.DockerfileInfo{},
							expectedRunImageImage:     "some-new-run-image",
							expectedRunImageReference: "some-new-run-image",
							expectedRunImageExtend:    true,
						},
						{
							before: func() {
								generator.RunMetadata = files.Run{
									Images: []files.RunImageForExport{
										{Image: "some-run-image"},
									},
								}
							},
							descCondition: "run metadata is provided and contains new run image",
							descResult:    "succeeds",
							aDockerfiles: []buildpack.DockerfileInfo{
								{
									ExtensionID: "A",
									Kind:        "run",
									Path:        runDockerfilePathA,
									WithBase:    "some-new-run-image",
									Extend:      false,
								},
							},
							bDockerfiles:              []buildpack.DockerfileInfo{},
							expectedRunImageImage:     "some-new-run-image",
							expectedRunImageReference: "some-new-run-image",
							expectedRunImageExtend:    false,
						},
						{
							before: func() {
								generator.RunMetadata = files.Run{
									Images: []files.RunImageForExport{
										{Image: "some-run-image"},
									},
								}
							},
							descCondition: "run metadata is provided and does not contain new run image",
							descResult:    "succeeds with warning",
							aDockerfiles: []buildpack.DockerfileInfo{
								{
									ExtensionID: "A",
									Kind:        "run",
									Path:        runDockerfilePathA,
									WithBase:    "some-other-run-image",
									Extend:      false,
								},
							},
							bDockerfiles:              []buildpack.DockerfileInfo{},
							expectedRunImageImage:     "some-other-run-image",
							expectedRunImageReference: "some-other-run-image",
							assertAfter: func() {
								h.AssertLogEntry(t, logHandler, "new runtime base image 'some-other-run-image' not found in run metadata")
							},
						},
					} {
						tc := tc
						when := when
						when(tc.descCondition, func() {
							if tc.before != nil {
								it.Before(tc.before)
							}

							it(tc.descResult, func() {
								// mock generate for extension A
								dirStore.EXPECT().LookupExt("A", "v1").Return(&extA, nil)
								executor.EXPECT().Generate(extA, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
									Dockerfiles: tc.aDockerfiles,
								}, nil)

								// mock generate for extension B
								dirStore.EXPECT().LookupExt("ext/B", "v2").Return(&extB, nil)
								executor.EXPECT().Generate(extB, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
									Dockerfiles: tc.bDockerfiles,
								}, nil)

								// do generate
								result, err := generator.Generate()
								if err == nil {
									h.AssertEq(t, result.AnalyzedMD.RunImage.Image, tc.expectedRunImageImage)
									h.AssertEq(t, result.AnalyzedMD.RunImage.Reference, tc.expectedRunImageReference)
									h.AssertEq(t, result.AnalyzedMD.RunImage.Extend, tc.expectedRunImageExtend)
								} else {
									t.Log(err)
									h.AssertError(t, err, tc.expectedErr)
								}

								if tc.assertAfter != nil {
									tc.assertAfter()
								}
							})
						})
					}
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
	})
}
