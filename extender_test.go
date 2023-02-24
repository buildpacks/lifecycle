package lifecycle_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/golang/mock/gomock"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/fake"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/extend"
	llog "github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/platform"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

func TestExtender(t *testing.T) {
	if runtime.GOOS != "windows" {
		spec.Run(t, "unit-new-extender", testExtenderFactory, spec.Report(report.Terminal{}))
		spec.Run(t, "unit-extender", testExtender, spec.Sequential(), spec.Report(report.Terminal{}))
	}
}

func testExtenderFactory(t *testing.T, when spec.G, it spec.S) {
	when("#NewExtender", func() {
		var (
			mockController    *gomock.Controller
			extenderFactory   *lifecycle.ExtenderFactory
			fakeAPIVerifier   *testmock.MockBuildpackAPIVerifier
			fakeConfigHandler *testmock.MockConfigHandler
			fakeDirStore      *testmock.MockDirStore
			logger            *log.Logger
			analyzedMD        platform.AnalyzedMetadata
			extender          *lifecycle.Extender
		)

		createExtender := func() {
			fakeConfigHandler.EXPECT().ReadAnalyzed("some-analyzed-path").Return(
				analyzedMD, nil,
			)
			fakeConfigHandler.EXPECT().ReadGroup("some-group-path").Return(
				[]buildpack.GroupElement{}, []buildpack.GroupElement{{ID: "A", Version: "v1", API: "0.9"}}, nil,
			)
			fakeDirStore.EXPECT().LookupExt("A", "v1").Return(&buildpack.ExtDescriptor{
				WithAPI: "0.9",
				Extension: buildpack.ExtInfo{
					BaseInfo: buildpack.BaseInfo{
						ID:      "A",
						Version: "v1",
					},
				},
			}, nil).AnyTimes()
			fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindExtension, "A@v1", "0.9", logger)

			fakeDockerfileApplier := testmock.NewMockDockerfileApplier(mockController)
			var err error
			extender, err = extenderFactory.NewExtender(
				"some-analyzed-path",
				"some-app-dir",
				"some-generated-dir",
				"some-group-path",
				"some-layers-dir",
				"some-platform-dir",
				7*(24*time.Hour),
				fakeDockerfileApplier,
				logger,
			)
			h.AssertNil(t, err)
		}

		it.Before(func() {
			mockController = gomock.NewController(t)
			fakeAPIVerifier = testmock.NewMockBuildpackAPIVerifier(mockController)
			fakeConfigHandler = testmock.NewMockConfigHandler(mockController)
			fakeDirStore = testmock.NewMockDirStore(mockController)
			extenderFactory = lifecycle.NewExtenderFactory(fakeAPIVerifier, fakeConfigHandler)

			logger = &log.Logger{Handler: &discard.Handler{}}
		})

		it.After(func() {
			mockController.Finish()
		})

		it("configures the extender", func() {
			analyzedMD = platform.AnalyzedMetadata{BuildImage: &platform.ImageIdentifier{Reference: "some-image-ref"}}
			createExtender()

			h.AssertEq(t, extender.AppDir, "some-app-dir")
			h.AssertEq(t, extender.GeneratedDir, "some-generated-dir")
			h.AssertEq(t, extender.ImageRef, "some-image-ref")
			h.AssertEq(t, extender.LayersDir, "some-layers-dir")
			h.AssertEq(t, extender.PlatformDir, "some-platform-dir")
			h.AssertEq(t, extender.CacheTTL, 7*(24*time.Hour))
			h.AssertEq(t, extender.Extensions, []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.9", Extension: true},
			})
		})

		when("build image is nil", func() {
			it("does not panic", func() {
				analyzedMD = platform.AnalyzedMetadata{}
				createExtender()
			})
		})
	})
}

func testExtender(t *testing.T, when spec.G, it spec.S) {
	when("Extend", func() {
		var (
			extender              *lifecycle.Extender
			fakeDockerfileApplier *testmock.MockDockerfileApplier
			mockCtrl              *gomock.Controller
			extendedDir           string
			generatedDir          string
			someFakeImage         *fake.FakeImage

			logger *log.Logger
		)

		it.Before(func() {
			mockCtrl = gomock.NewController(t)

			var err error
			extendedDir, err = os.MkdirTemp("", "lifecycle")
			h.AssertNil(t, err)
			generatedDir, err = os.MkdirTemp("", "lifecycle")
			h.AssertNil(t, err)

			someFakeImage = &fake.FakeImage{}

			providedExtensions := []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: api.Buildpack.Latest().String(), Homepage: "A Homepage"},
				{ID: "B", Version: "v2", API: api.Buildpack.Latest().String()},
			}
			fakeDockerfileApplier = testmock.NewMockDockerfileApplier(mockCtrl)
			extender = &lifecycle.Extender{
				AppDir:            "some-app-dir",
				ImageRef:          "some-image-tag@sha256:9412cff392ca11c0d7b9df015808c4e40aff218fbe324df6490b9552ba82be38",
				ExtendedDir:       extendedDir,
				GeneratedDir:      generatedDir,
				LayersDir:         "some-layers-dir",
				PlatformDir:       "some-platform-dir",
				CacheTTL:          7 * (24 * time.Hour),
				DockerfileApplier: fakeDockerfileApplier,
				Extensions:        providedExtensions,
			}

			logger = &log.Logger{Handler: &discard.Handler{}}
		})

		it.After(func() {
			_ = os.RemoveAll(generatedDir)
			mockCtrl.Finish()
		})

		when("Dockerfile kind build", func() {
			it("applies the Dockerfile with the expected args and opts", func() {
				h.Mkdir(t, filepath.Join(generatedDir, "build", "B"))
				h.Mkfile(t, "some dockerfile content", filepath.Join(generatedDir, "build", "B", "Dockerfile"))
				h.Mkfile(t, "[[build.args]]\nname=\"arg1\"\nvalue=\"value1\"", filepath.Join(generatedDir, "build", "B", "extend-config.toml"))

				// TODO: expectedDockerfileA
				expectedDockerfileB := extend.Dockerfile{
					Path: filepath.Join(generatedDir, "build", "B", "Dockerfile"),
					Args: []extend.Arg{{Name: "arg1", Value: "value1"}},
				}

				fakeDockerfileApplier.EXPECT().ImageFor(extender.ImageRef).Return(someFakeImage, nil)
				fakeDockerfileApplier.EXPECT().Apply(
					gomock.Any(),
					someFakeImage,
					gomock.Any(), // TODO: make concrete
					logger,
				).DoAndReturn(
					func(dockerfile extend.Dockerfile, toBaseImage v1.Image, withBuildOptions extend.Options, logger llog.Logger) (v1.Image, error) {
						h.AssertEq(t, dockerfile.Path, expectedDockerfileB.Path)
						h.AssertEq(t, len(dockerfile.Args), 4) // TODO: assert build_id, user_id, group_id
						h.AssertEq(t, dockerfile.Args[3], expectedDockerfileB.Args[0])

						return someFakeImage, nil
					})
				someFakeImage.ConfigFileReturns(&v1.ConfigFile{Config: v1.Config{
					Env: []string{"SOME_VAR=some-val"},
				}}, nil)
				fakeDockerfileApplier.EXPECT().Cleanup().Return(nil)

				h.AssertNil(t, extender.Extend("build", logger))
				h.AssertEq(t, os.Getenv("SOME_VAR"), "some-val")
				h.AssertNil(t, os.Unsetenv("SOME_VAR"))
			})
		})

		when("Dockerfile kind run", func() {
			it.Focus("applies the Dockerfile with the expected args and opts", func() {
				h.Mkdir(t, filepath.Join(generatedDir, "run", "B"))
				h.Mkfile(t, "some dockerfile content", filepath.Join(generatedDir, "run", "B", "Dockerfile"))
				h.Mkfile(t, "[[build.args]]\nname=\"arg1\"\nvalue=\"value1\"", filepath.Join(generatedDir, "run", "B", "extend-config.toml"))

				// TODO: expectedDockerfileA
				expectedDockerfileB := extend.Dockerfile{
					Path: filepath.Join(generatedDir, "run", "B", "Dockerfile"),
					Args: []extend.Arg{{Name: "arg1", Value: "value1"}},
				}

				fakeDockerfileApplier.EXPECT().ImageFor(extender.ImageRef).Return(someFakeImage, nil)
				someFakeImage.ManifestReturns(&v1.Manifest{Layers: []v1.Descriptor{}}, nil)
				fakeDockerfileApplier.EXPECT().Apply(
					gomock.Any(),
					someFakeImage,
					gomock.Any(),
					logger,
				).DoAndReturn(
					func(dockerfile extend.Dockerfile, toBaseImage v1.Image, withBuildOptions extend.Options, logger llog.Logger) (v1.Image, error) {
						h.AssertEq(t, dockerfile.Path, expectedDockerfileB.Path)
						t.Log(dockerfile.Args)
						h.AssertEq(t, len(dockerfile.Args), 4) // TODO: assert build_id, user_id, group_id
						h.AssertEq(t, dockerfile.Args[3], expectedDockerfileB.Args[0])

						return someFakeImage, nil
					})
				// save selective
				imageHash := v1.Hash{Algorithm: "sha256", Hex: "some-image-hex"}
				someFakeImage.DigestReturns(imageHash, nil)
				someFakeImage.ConfigNameReturns(v1.Hash{Algorithm: "sha256", Hex: "some-config-hex"}, nil)
				fakeDockerfileApplier.EXPECT().Cleanup().Return(nil)

				h.AssertNil(t, extender.Extend("run", logger))
				outputImagePath := filepath.Join(extendedDir, imageHash.String())
				h.AssertPathExists(t, outputImagePath)
				fis, err := os.ReadDir(outputImagePath)
				h.AssertNil(t, err)
				h.AssertEq(t, len(fis), 3)
			})
		})
	})
}
