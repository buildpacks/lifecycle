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
				buildpack.Group{Group: []buildpack.GroupElement{
					{ID: "A", Version: "v1", API: "0.2"},
					{ID: "A", Version: "v1", API: "0.9", Extension: true},
				}},
				"some-output-dir",
				providedPlan,
				"some-platform-dir",
				logger,
			)
			h.AssertNil(t, err)

			h.AssertEq(t, generator.AppDir, "some-app-dir")
			h.AssertNotNil(t, generator.DirStore)
			h.AssertEq(t, generator.Group, buildpack.Group{Group: []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.9", Extension: true},
			}})
			h.AssertEq(t, generator.OutputDir, filepath.Join("some-output-dir", "generated"))
			h.AssertEq(t, generator.Logger, logger)
			h.AssertEq(t, generator.Plan, providedPlan)
			h.AssertEq(t, generator.PlatformDir, "some-platform-dir")
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
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		outputDir = filepath.Join(tmpDir, "output")
		appDir = filepath.Join(outputDir, "app")
		platformDir = filepath.Join(tmpDir, "platform")
		h.Mkdir(t, outputDir, appDir, filepath.Join(platformDir, "env"))

		providedGroup := buildpack.Group{
			Group: []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: api.Buildpack.Latest().String(), Extension: true, Homepage: "A Homepage"},
				{ID: "B", Version: "v2", API: api.Buildpack.Latest().String(), Extension: true},
			},
		}
		generator = &lifecycle.Generator{
			AppDir:      appDir,
			DirStore:    dirStore,
			Group:       providedGroup,
			OutputDir:   outputDir,
			Logger:      &log.Logger{Handler: logHandler},
			Plan:        platform.BuildPlan{},
			PlatformDir: platformDir,
			Out:         stdout,
			Err:         stderr,
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
			extA.EXPECT().Build(expectedPlanA, generator.GenerateConfig(), gomock.Any()).Return(buildpack.BuildResult{
				MetRequires: []string{"some-dep"},
			}, nil)
			dirStore.EXPECT().Lookup(buildpack.KindExtension, "B", "v2").Return(extB, nil)
			expectedPlanB := buildpack.Plan{Entries: []buildpack.Require{
				{Name: "some-unmet-dep", Version: "v2"},
				{Name: "other-dep", Version: "v4"},
			}}
			extB.EXPECT().Build(expectedPlanB, generator.GenerateConfig(), gomock.Any())

			_, err := generator.Generate()
			h.AssertNil(t, err)
		})

		when("generated metadata", func() {
			when("dockerfiles", func() {
				it("aggregates dockerfiles from each extension", func() {
					h.AssertNil(t, os.MkdirAll(filepath.Join(outputDir, "A"), 0755))
					dockerfilePath1 := filepath.Join(outputDir, "A", "run.Dockerfile")
					h.Mkfile(t, `FROM some-run-image`, dockerfilePath1)
					bRootDir := filepath.Join(tmpDir, "extensions", "B", "v2")
					h.AssertNil(t, os.MkdirAll(bRootDir, 0755))
					dockerfilePath2 := filepath.Join(bRootDir, "run.Dockerfile")
					h.Mkfile(t, `FROM other-run-image`, dockerfilePath2)

					extA := testmock.NewMockBuildModule(mockCtrl)
					dirStore.EXPECT().Lookup(buildpack.KindExtension, "A", "v1").Return(extA, nil)
					extA.EXPECT().Build(gomock.Any(), generator.GenerateConfig(), gomock.Any()).Return(buildpack.BuildResult{
						Dockerfiles: []buildpack.Dockerfile{
							{ExtensionID: "A", Path: dockerfilePath1, Kind: "run"},
						},
					}, nil)
					extB := testmock.NewMockBuildModule(mockCtrl)
					dirStore.EXPECT().Lookup(buildpack.KindExtension, "B", "v2").Return(extB, nil)
					extB.EXPECT().Build(gomock.Any(), generator.GenerateConfig(), gomock.Any()).Return(buildpack.BuildResult{
						Dockerfiles: []buildpack.Dockerfile{
							{ExtensionID: "B", Path: dockerfilePath2, Kind: "run"},
						},
					}, nil)

					metadata, err := generator.Generate()
					h.AssertNil(t, err)
					h.AssertEq(t, metadata.Dockerfiles, []buildpack.Dockerfile{
						{ExtensionID: "A", Path: filepath.Join(outputDir, "A", "run.Dockerfile"), Kind: "run"},
						{ExtensionID: "B", Path: filepath.Join(outputDir, "B", "run.Dockerfile"), Kind: "run"},
					})

					t.Log("copies Dockerfiles to the correct locations")
					contents1 := h.MustReadFile(t, filepath.Join(outputDir, "A", "run.Dockerfile"))
					h.AssertEq(t, string(contents1), `FROM some-run-image`)
					contents2 := h.MustReadFile(t, filepath.Join(outputDir, "B", "run.Dockerfile"))
					h.AssertEq(t, string(contents2), `FROM other-run-image`)
				})
			})
		})

		when("extension generate failed", func() {
			when("first extension fails", func() {
				it("errors", func() {
					bpA := testmock.NewMockBuildModule(mockCtrl)
					dirStore.EXPECT().Lookup(buildpack.KindExtension, "A", "v1").Return(bpA, nil)
					bpA.EXPECT().Build(gomock.Any(), generator.GenerateConfig(), gomock.Any()).Return(buildpack.BuildResult{}, errors.New("some error"))

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
					bpA.EXPECT().Build(gomock.Any(), generator.GenerateConfig(), gomock.Any()).Return(buildpack.BuildResult{}, nil)
					bpB := testmock.NewMockBuildModule(mockCtrl)
					dirStore.EXPECT().Lookup(buildpack.KindExtension, "B", "v2").Return(bpB, nil)
					bpB.EXPECT().Build(gomock.Any(), generator.GenerateConfig(), gomock.Any()).Return(buildpack.BuildResult{}, errors.New("some error"))

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
