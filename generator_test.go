package lifecycle_test

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
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
	spec.Run(t, "unit-new-generator", testGeneratorFactory, spec.Report(report.Terminal{}))
	spec.Run(t, "unit-generator", testGenerator, spec.Report(report.Terminal{}))
}

func testGeneratorFactory(t *testing.T, when spec.G, it spec.S) {
	when("#NewGenerator", func() {
		var (
			generatorFactory *lifecycle.GeneratorFactory
			fakeAPIVerifier  *testmock.MockBuildpackAPIVerifier
			fakeDirStore     *testmock.MockDirStore
			logger           *log.Logger
			mockController   *gomock.Controller
			stdout, stderr   *bytes.Buffer
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			fakeAPIVerifier = testmock.NewMockBuildpackAPIVerifier(mockController)
			fakeDirStore = testmock.NewMockDirStore(mockController)
			logger = &log.Logger{Handler: &discard.Handler{}}

			generatorFactory = lifecycle.NewGeneratorFactory(
				fakeAPIVerifier,
				fakeDirStore,
			)
		})

		it.After(func() {
			mockController.Finish()
		})

		it("configures the generator", func() {
			fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindExtension, "A@v1", "0.9", logger)

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
				"some-app-dir",
				[]buildpack.GroupElement{
					{ID: "A", Version: "v1", API: "0.9"},
				},
				"some-output-dir",
				providedPlan,
				"some-platform-dir",
				stdout, stderr,
				logger,
			)
			h.AssertNil(t, err)

			h.AssertEq(t, generator.AppDir, "some-app-dir")
			h.AssertNotNil(t, generator.DirStore)
			h.AssertEq(t, generator.Extensions, []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.9"},
			})
			h.AssertEq(t, generator.GeneratedDir, "some-output-dir")
			h.AssertEq(t, generator.Logger, logger)
			h.AssertEq(t, generator.Plan, providedPlan)
			h.AssertEq(t, generator.PlatformDir, "some-platform-dir")
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
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		h.AssertNil(t, err)
		generatedDir = filepath.Join(tmpDir, "output")
		appDir = filepath.Join(generatedDir, "app")
		platformDir = filepath.Join(tmpDir, "platform")
		h.Mkdir(t, generatedDir, appDir, filepath.Join(platformDir, "env"))
		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}

		generator = &lifecycle.Generator{
			AppDir:   appDir,
			DirStore: dirStore,
			Executor: executor,
			Extensions: []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: api.Buildpack.Latest().String(), Homepage: "A Homepage"},
				{ID: "B", Version: "v2", API: api.Buildpack.Latest().String()},
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
		it("provides a subset of the build plan to each extension", func() {
			generator.Plan = platform.BuildPlan{
				Entries: []platform.BuildPlanEntry{
					{
						Providers: []buildpack.GroupElement{
							{ID: "A", Version: "v1"}, // not provided to any extension
						},
						Requires: []buildpack.Require{
							{Name: "buildpack-dep", Version: "v1"},
						},
					},
					{
						Providers: []buildpack.GroupElement{
							{ID: "A", Version: "v1", Extension: true},
							{ID: "B", Version: "v2", Extension: true},
						},
						Requires: []buildpack.Require{
							{Name: "some-dep", Version: "v1"}, // not provided to B because it is met
						},
					},
					{
						Providers: []buildpack.GroupElement{
							{ID: "A", Version: "v1", Extension: true},
							{ID: "B", Version: "v2", Extension: true},
						},
						Requires: []buildpack.Require{
							{Name: "some-unmet-dep", Version: "v2"}, // provided to B because it is unmet
						},
					},
					{
						Providers: []buildpack.GroupElement{
							{ID: "B", Version: "v2", Extension: true},
						},
						Requires: []buildpack.Require{
							{Name: "other-dep", Version: "v4"}, // only provided to B
						},
					},
				},
			}
			expectedInputs := buildpack.GenerateInputs{
				AppDir:      appDir,
				PlatformDir: platformDir,
				// OutputDir is ephemeral directory
			}

			extA := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
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
					return buildpack.GenerateOutputs{MetRequires: []string{"some-dep"}}, nil
				})

			extB := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
			dirStore.EXPECT().LookupExt("B", "v2").Return(&extB, nil)
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
					return buildpack.GenerateOutputs{}, nil
				})

			_, err := generator.Generate()
			h.AssertNil(t, err)
		})

		it("aggregates dockerfiles from each extension", func() {
			// Extension A outputs a run.Dockerfile to the provided output directory when invoked.
			extA := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "A", Version: "v1"}}}
			dirStore.EXPECT().LookupExt("A", "v1").Return(&extA, nil)
			executor.EXPECT().Generate(extA, gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ buildpack.ExtDescriptor, inputs buildpack.GenerateInputs, _ llog.Logger) (buildpack.GenerateOutputs, error) {
					// check inputs
					h.AssertEq(t, inputs.AppDir, generator.AppDir)
					h.AssertEq(t, inputs.PlatformDir, generator.PlatformDir)

					// create fixture
					h.Mkdir(t, filepath.Join(inputs.OutputDir, "A"))
					dockerfilePath1 := filepath.Join(inputs.OutputDir, "A", "run.Dockerfile")
					h.Mkfile(t, `FROM some-run-image`, dockerfilePath1)

					return buildpack.GenerateOutputs{
						Dockerfiles: []buildpack.DockerfileInfo{
							{ExtensionID: "A", Path: dockerfilePath1, Kind: "run"},
						},
					}, nil
				},
			)

			// Extension B has a pre-populated root directory.
			extB := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
			bRootDir := filepath.Join(tmpDir, "some-b-root-dir")
			h.Mkdir(t, bRootDir)
			bDockerfilePath := filepath.Join(bRootDir, "run.Dockerfile")
			h.Mkfile(t, `FROM other-run-image`, bDockerfilePath)
			dirStore.EXPECT().LookupExt("B", "v2").Return(&extB, nil)
			executor.EXPECT().Generate(extB, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
				Dockerfiles: []buildpack.DockerfileInfo{
					{ExtensionID: "B", Path: bDockerfilePath, Kind: "run"},
				},
			}, nil)

			// Extension C has a pre-populated root directory.
			extC := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "C", Version: "v1"}}}
			cRootDir := filepath.Join(tmpDir, "some-c-root-dir")
			h.Mkdir(t, cRootDir)
			cDockerfilePath := filepath.Join(cRootDir, "build.Dockerfile")
			h.Mkfile(t, `some-build.Dockerfile-content`, cDockerfilePath)
			h.Mkfile(t, `some-extend-config-content`, filepath.Join(cRootDir, "extend-config.toml"))
			dirStore.EXPECT().LookupExt("C", "v1").Return(&extC, nil)
			executor.EXPECT().Generate(extC, gomock.Any(), gomock.Any()).Return(buildpack.GenerateOutputs{
				Dockerfiles: []buildpack.DockerfileInfo{
					{ExtensionID: "C", Path: cDockerfilePath, Kind: "build"},
				},
			}, nil)

			generator.Extensions = append(generator.Extensions, buildpack.GroupElement{ID: "C", Version: "v1", API: api.Buildpack.Latest().String()})
			result, err := generator.Generate()
			h.AssertNil(t, err)

			t.Log("copies Dockerfiles to the correct locations")
			aContents := h.MustReadFile(t, filepath.Join(generatedDir, "run", "A", "Dockerfile"))
			h.AssertEq(t, string(aContents), `FROM some-run-image`)
			bContents := h.MustReadFile(t, filepath.Join(generatedDir, "run", "B", "Dockerfile"))
			h.AssertEq(t, string(bContents), `FROM other-run-image`)
			cContents := h.MustReadFile(t, filepath.Join(generatedDir, "build", "C", "Dockerfile"))
			h.AssertEq(t, string(cContents), `some-build.Dockerfile-content`)

			t.Log("copies the extend-config.toml if exists")
			configContents := h.MustReadFile(t, filepath.Join(generatedDir, "build", "C", "extend-config.toml"))
			h.AssertEq(t, string(configContents), `some-extend-config-content`)

			t.Log("does not pollute the output directory")
			h.AssertPathDoesNotExist(t, filepath.Join(generatedDir, "A", "run.Dockerfile"))

			t.Log("returns the correct run image")
			h.AssertEq(t, result.RunImage, "other-run-image")
		})

		it("validates build.Dockerfiles", func() {
			// TODO: validate the following conditions:
			/*
				build.Dockerfiles:
				- MUST begin with:
				```bash
				ARG base_image
				FROM ${base_image}
				```
				- MUST NOT contain any other `FROM` instructions
				- MAY contain `ADD`, `ARG`, `COPY`, `ENV`, `LABEL`, `RUN`, `SHELL`, `USER`, and `WORKDIR` instructions
				- SHOULD NOT contain any other instructions
			*/
		})

		it("validates run.Dockerfiles", func() {
			// TODO: validate the following conditions:
			/*
				run.Dockerfiles:
				- MAY contain a single `FROM` instruction
				- MUST NOT contain any other instructions
			*/
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
					extB := buildpack.ExtDescriptor{Extension: buildpack.ExtInfo{BaseInfo: buildpack.BaseInfo{ID: "B", Version: "v1"}}}
					dirStore.EXPECT().LookupExt("B", "v2").Return(&extB, nil)
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
