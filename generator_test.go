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
			h.AssertEq(t, generator.OutputDir, "some-output-dir")
			h.AssertEq(t, generator.Logger, logger)
			h.AssertEq(t, generator.Plan, providedPlan)
			h.AssertEq(t, generator.PlatformDir, "some-platform-dir")
			h.AssertEq(t, generator.Stdout, stdout)
			h.AssertEq(t, generator.Stderr, stderr)
		})
	})
}

func testGenerator(t *testing.T, when spec.G, it spec.S) {
	var (
		generator      *lifecycle.Generator
		mockCtrl       *gomock.Controller
		dirStore       *testmock.MockDirStore
		stdout, stderr *bytes.Buffer
		appDir         string
		outputDir      string
		platformDir    string
		tmpDir         string

		logHandler = memory.New()
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		dirStore = testmock.NewMockDirStore(mockCtrl)
		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}

		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		h.AssertNil(t, err)
		outputDir = filepath.Join(tmpDir, "output")
		appDir = filepath.Join(outputDir, "app")
		platformDir = filepath.Join(tmpDir, "platform")
		h.Mkdir(t, outputDir, appDir, filepath.Join(platformDir, "env"))

		providedExtensions := []buildpack.GroupElement{
			{ID: "A", Version: "v1", API: api.Buildpack.Latest().String(), Homepage: "A Homepage"},
			{ID: "B", Version: "v2", API: api.Buildpack.Latest().String()},
		}
		generator = &lifecycle.Generator{
			AppDir:      appDir,
			DirStore:    dirStore,
			Extensions:  providedExtensions,
			Logger:      &log.Logger{Handler: logHandler},
			OutputDir:   outputDir,
			Plan:        platform.BuildPlan{},
			PlatformDir: platformDir,
			Stderr:      stderr,
			Stdout:      stdout,
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
		mockCtrl.Finish()
	})

	when(".Generate", func() {
		it("provides a subset of the build plan to each extension", func() {
			providedPlan := platform.BuildPlan{
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
			generator.Plan = providedPlan

			extA := testmock.NewMockBuildModule(mockCtrl)
			extB := testmock.NewMockBuildModule(mockCtrl)
			dirStore.EXPECT().Lookup(buildpack.KindExtension, "A", "v1").Return(extA, nil)
			expectedPlanA := buildpack.Plan{Entries: []buildpack.Require{
				{Name: "some-dep", Version: "v1"},
				{Name: "some-unmet-dep", Version: "v2"},
			}}
			extA.EXPECT().Build(expectedPlanA, gomock.Any(), gomock.Any()).Return(buildpack.BuildResult{
				MetRequires: []string{"some-dep"},
			}, nil)
			dirStore.EXPECT().Lookup(buildpack.KindExtension, "B", "v2").Return(extB, nil)
			expectedPlanB := buildpack.Plan{Entries: []buildpack.Require{
				{Name: "some-unmet-dep", Version: "v2"},
				{Name: "other-dep", Version: "v4"},
			}}
			extB.EXPECT().Build(expectedPlanB, gomock.Any(), gomock.Any())

			_, err := generator.Generate()
			h.AssertNil(t, err)
		})

		it("aggregates dockerfiles from each extension", func() {
			// Extension A outputs a run.Dockerfile to the provided output directory when invoked.
			extA := testmock.NewMockBuildModule(mockCtrl)
			dirStore.EXPECT().Lookup(buildpack.KindExtension, "A", "v1").Return(extA, nil)
			extA.EXPECT().Build(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
				func(_ buildpack.Plan, config buildpack.BuildConfig, _ buildpack.BuildEnv) (buildpack.BuildResult, error) {
					// check config
					h.AssertEq(t, config.AppDir, generator.AppDir)
					h.AssertEq(t, config.PlatformDir, generator.PlatformDir)

					// create fixture
					h.Mkdir(t, filepath.Join(config.OutputParentDir, "A"))
					dockerfilePath1 := filepath.Join(config.OutputParentDir, "A", "run.Dockerfile")
					h.Mkfile(t, `FROM some-run-image`, dockerfilePath1)

					return buildpack.BuildResult{
						Dockerfiles: []buildpack.Dockerfile{
							{ExtensionID: "A", Path: dockerfilePath1, Kind: "run"},
						},
					}, nil
				})

			// Extension B has a pre-populated root directory.
			extB := testmock.NewMockBuildModule(mockCtrl)
			bRootDir := filepath.Join(tmpDir, "extensions", "B", "v2")
			h.Mkdir(t, bRootDir)
			bDockerfilePath := filepath.Join(bRootDir, "run.Dockerfile")
			h.Mkfile(t, `FROM other-run-image`, bDockerfilePath)
			dirStore.EXPECT().Lookup(buildpack.KindExtension, "B", "v2").Return(extB, nil)
			extB.EXPECT().Build(gomock.Any(), gomock.Any(), gomock.Any()).Return(buildpack.BuildResult{
				Dockerfiles: []buildpack.Dockerfile{
					{ExtensionID: "B", Path: bDockerfilePath, Kind: "run"},
				},
			}, nil)

			// Extension C has a pre-populated root directory.
			extC := testmock.NewMockBuildModule(mockCtrl)
			cRootDir := filepath.Join(tmpDir, "extensions", "C", "v1") // TODO (before drafting): this should be nested under `generated`
			h.Mkdir(t, cRootDir)
			cDockerfilePath := filepath.Join(cRootDir, "build.Dockerfile")
			h.Mkfile(t, `some-build.Dockerfile-content`, cDockerfilePath)
			dirStore.EXPECT().Lookup(buildpack.KindExtension, "C", "v1").Return(extC, nil)
			extC.EXPECT().Build(gomock.Any(), gomock.Any(), gomock.Any()).Return(buildpack.BuildResult{
				Dockerfiles: []buildpack.Dockerfile{
					{ExtensionID: "C", Path: cDockerfilePath, Kind: "build"},
				},
			}, nil)

			generator.Extensions = append(generator.Extensions, buildpack.GroupElement{ID: "C", Version: "v1", API: api.Buildpack.Latest().String()})
			result, err := generator.Generate()
			h.AssertNil(t, err)

			t.Log("copies Dockerfiles to the correct locations")
			aContents := h.MustReadFile(t, filepath.Join(outputDir, "run", "A", "Dockerfile"))
			h.AssertEq(t, string(aContents), `FROM some-run-image`)
			bContents := h.MustReadFile(t, filepath.Join(outputDir, "run", "B", "Dockerfile"))
			h.AssertEq(t, string(bContents), `FROM other-run-image`)
			cContents := h.MustReadFile(t, filepath.Join(outputDir, "build", "C", "Dockerfile"))
			h.AssertEq(t, string(cContents), `some-build.Dockerfile-content`)

			t.Log("does not pollute the output directory")
			h.AssertPathDoesNotExist(t, filepath.Join(outputDir, "A", "run.Dockerfile"))

			t.Log("returns the correct run image")
			h.AssertEq(t, result.RunImage, "other-run-image")
		})

		when("extension generate failed", func() {
			when("first extension fails", func() {
				it("errors", func() {
					bpA := testmock.NewMockBuildModule(mockCtrl)
					dirStore.EXPECT().Lookup(buildpack.KindExtension, "A", "v1").Return(bpA, nil)
					bpA.EXPECT().Build(gomock.Any(), gomock.Any(), gomock.Any()).Return(buildpack.BuildResult{}, errors.New("some error"))

					if _, err := generator.Generate(); err == nil {
						t.Fatal("Expected error.\n")
					} else if !strings.Contains(err.Error(), "some error") {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})
			})

			when("later extension fails", func() {
				it("errors", func() {
					bpA := testmock.NewMockBuildModule(mockCtrl)
					dirStore.EXPECT().Lookup(buildpack.KindExtension, "A", "v1").Return(bpA, nil)
					bpA.EXPECT().Build(gomock.Any(), gomock.Any(), gomock.Any()).Return(buildpack.BuildResult{}, nil)
					bpB := testmock.NewMockBuildModule(mockCtrl)
					dirStore.EXPECT().Lookup(buildpack.KindExtension, "B", "v2").Return(bpB, nil)
					bpB.EXPECT().Build(gomock.Any(), gomock.Any(), gomock.Any()).Return(buildpack.BuildResult{}, errors.New("some error"))

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
