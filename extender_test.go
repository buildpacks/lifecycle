package lifecycle_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/golang/mock/gomock"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/fake"
	"github.com/google/uuid"
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
			fakeConfigHandler.EXPECT().ReadAnalyzed("some-analyzed-path", logger).Return(
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
				"some-extended-dir",
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
			h.AssertEq(t, extender.ExtendedDir, "some-extended-dir")
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
			_ = os.RemoveAll(extendedDir)
			_ = os.RemoveAll(generatedDir)
			mockCtrl.Finish()
		})

		when("build base image", func() {
			it("applies the Dockerfile with the expected args and opts", func() {
				expectedDockerfileA := extend.Dockerfile{
					Path: filepath.Join(generatedDir, "build", "A", "Dockerfile"),
					Args: []extend.Arg{{Name: "argA", Value: "valueA"}},
				}
				h.Mkdir(t, filepath.Join(generatedDir, "build", "A"))
				h.Mkfile(t, "some dockerfile content", filepath.Join(generatedDir, "build", "A", "Dockerfile"))
				buf := new(bytes.Buffer)
				data := extend.Config{Build: extend.BuildConfig{Args: expectedDockerfileA.Args}}
				h.AssertNil(t, toml.NewEncoder(buf).Encode(data))
				h.Mkfile(t, buf.String(), filepath.Join(generatedDir, "build", "A", "extend-config.toml"))

				expectedDockerfileB := extend.Dockerfile{
					Path: filepath.Join(generatedDir, "build", "B", "Dockerfile"),
					Args: []extend.Arg{{Name: "argB", Value: "valueB"}},
				}
				h.Mkdir(t, filepath.Join(generatedDir, "build", "B"))
				h.Mkfile(t, "some dockerfile content", filepath.Join(generatedDir, "build", "B", "Dockerfile"))
				buf = new(bytes.Buffer)
				data = extend.Config{Build: extend.BuildConfig{Args: expectedDockerfileB.Args}}
				h.AssertNil(t, toml.NewEncoder(buf).Encode(data))
				h.Mkfile(t, buf.String(), filepath.Join(generatedDir, "build", "B", "extend-config.toml"))

				fakeDockerfileApplier.EXPECT().ImageFor(extender.ImageRef).Return(someFakeImage, nil)
				firstConfig := &v1.ConfigFile{Config: v1.Config{
					User: "1234:5678",
				}}
				someFakeImage.ConfigFileReturnsOnCall(0, firstConfig, nil)

				// first dockerfile

				fakeDockerfileApplier.EXPECT().Apply(
					gomock.Any(),
					someFakeImage,
					extend.Options{
						BuildContext: "some-app-dir",
						IgnorePaths:  []string{"some-app-dir", "some-layers-dir", "some-platform-dir"},
						CacheTTL:     7 * (24 * time.Hour),
					},
					logger,
				).DoAndReturn(
					func(dockerfile extend.Dockerfile, toBaseImage v1.Image, withBuildOptions extend.Options, logger llog.Logger) (v1.Image, error) {
						h.AssertEq(t, dockerfile.Path, expectedDockerfileA.Path)
						h.AssertEq(t, len(dockerfile.Args), 4)
						h.AssertEq(t, dockerfile.Args[0].Name, "build_id")
						_, err := uuid.Parse(dockerfile.Args[0].Value)
						h.AssertNil(t, err)
						h.AssertEq(t, dockerfile.Args[1].Name, "user_id")
						h.AssertEq(t, dockerfile.Args[1].Value, "1234")
						h.AssertEq(t, dockerfile.Args[2].Name, "group_id")
						h.AssertEq(t, dockerfile.Args[2].Value, "5678")
						h.AssertEq(t, dockerfile.Args[3], expectedDockerfileA.Args[0])

						return someFakeImage, nil
					})
				secondConfig := &v1.ConfigFile{Config: v1.Config{
					User: "2345:6789",
					Env:  []string{"SOME_VAR=some-val"},
				}}
				someFakeImage.ConfigFileReturnsOnCall(1, secondConfig, nil)

				// second dockerfile

				fakeDockerfileApplier.EXPECT().Apply(
					gomock.Any(),
					someFakeImage,
					extend.Options{
						BuildContext: "some-app-dir",
						IgnorePaths:  []string{"some-app-dir", "some-layers-dir", "some-platform-dir"},
						CacheTTL:     7 * (24 * time.Hour),
					},
					logger,
				).DoAndReturn(
					func(dockerfile extend.Dockerfile, toBaseImage v1.Image, withBuildOptions extend.Options, logger llog.Logger) (v1.Image, error) {
						h.AssertEq(t, dockerfile.Path, expectedDockerfileB.Path)
						h.AssertEq(t, len(dockerfile.Args), 4)
						h.AssertEq(t, dockerfile.Args[0].Name, "build_id")
						_, err := uuid.Parse(dockerfile.Args[0].Value)
						h.AssertNil(t, err)
						h.AssertEq(t, dockerfile.Args[1].Name, "user_id")
						h.AssertEq(t, dockerfile.Args[1].Value, "2345")
						h.AssertEq(t, dockerfile.Args[2].Name, "group_id")
						h.AssertEq(t, dockerfile.Args[2].Value, "6789")
						h.AssertEq(t, dockerfile.Args[3], expectedDockerfileB.Args[0])

						return someFakeImage, nil
					})
				someFakeImage.ConfigFileReturnsOnCall(2, secondConfig, nil)

				someFakeImage.ConfigFileReturnsOnCall(3, secondConfig, nil)
				fakeDockerfileApplier.EXPECT().Cleanup().Return(nil)

				h.AssertNil(t, extender.Extend("build", logger))
				h.AssertEq(t, os.Getenv("SOME_VAR"), "some-val")
				h.AssertNil(t, os.Unsetenv("SOME_VAR"))
			})
		})

		when("run base image", func() {
			type testCase struct {
				firstDockerfileRebasable  bool
				secondDockerfileRebasable bool
				expectedImageSHA          string
			}
			var (
				rebasableSHA    = "sha256:2407b98f3bcee33d24f47fc3d7beaf0928b3fa799a8a84aa9f9f239fcded6e62"
				notRebasableSHA = "sha256:cc3b90b798b76e7d41de01d4f02f2917694ef0b09d0d923314c8d6c31c4ae8e9"
			)
			for _, tc := range []testCase{
				{
					firstDockerfileRebasable:  true,
					secondDockerfileRebasable: true,
					expectedImageSHA:          rebasableSHA,
				},
				{
					firstDockerfileRebasable:  true,
					secondDockerfileRebasable: false,
					expectedImageSHA:          notRebasableSHA,
				},
				{
					firstDockerfileRebasable:  false,
					secondDockerfileRebasable: true,
					expectedImageSHA:          notRebasableSHA,
				},
				{
					firstDockerfileRebasable:  false,
					secondDockerfileRebasable: false,
					expectedImageSHA:          notRebasableSHA,
				},
			} {
				tc := tc
				when := when
				desc := func(b bool) string {
					if b {
						return "rebasable"
					}
					return "not rebasable"
				}
				when(fmt.Sprintf("first Dockerfile is %s, second Dockerfile is %s", desc(tc.firstDockerfileRebasable), desc(tc.secondDockerfileRebasable)), func() {
					it("applies the Dockerfile with the expected args and opts", func() {
						expectedDockerfileA := extend.Dockerfile{
							Path: filepath.Join(generatedDir, "run", "A", "Dockerfile"),
							Args: []extend.Arg{{Name: "argA", Value: "valueA"}},
						}
						h.Mkdir(t, filepath.Join(generatedDir, "run", "A"))
						h.Mkfile(t, "some dockerfile content", filepath.Join(generatedDir, "run", "A", "Dockerfile"))
						buf := new(bytes.Buffer)
						data := extend.Config{Run: extend.BuildConfig{Args: expectedDockerfileA.Args}}
						h.AssertNil(t, toml.NewEncoder(buf).Encode(data))
						h.Mkfile(t, buf.String(), filepath.Join(generatedDir, "run", "A", "extend-config.toml"))

						expectedDockerfileB := extend.Dockerfile{
							Path: filepath.Join(generatedDir, "run", "B", "Dockerfile"),
							Args: []extend.Arg{{Name: "argB", Value: "valueB"}},
						}
						h.Mkdir(t, filepath.Join(generatedDir, "run", "B"))
						h.Mkfile(t, "some dockerfile content", filepath.Join(generatedDir, "run", "B", "Dockerfile"))
						buf = new(bytes.Buffer)
						data = extend.Config{Run: extend.BuildConfig{Args: expectedDockerfileB.Args}}
						h.AssertNil(t, toml.NewEncoder(buf).Encode(data))
						h.Mkfile(t, buf.String(), filepath.Join(generatedDir, "run", "B", "extend-config.toml"))

						fakeDockerfileApplier.EXPECT().ImageFor(extender.ImageRef).Return(someFakeImage, nil)
						someFakeImage.ManifestReturns(&v1.Manifest{Layers: []v1.Descriptor{}}, nil)
						someFakeImage.ConfigFileReturnsOnCall(0, &v1.ConfigFile{Config: v1.Config{
							User: "1234:5678",
						}}, nil)

						// first dockerfile

						fakeDockerfileApplier.EXPECT().Apply(
							gomock.Any(),
							someFakeImage,
							extend.Options{
								BuildContext: "some-app-dir",
								IgnorePaths:  []string{"some-app-dir", "some-layers-dir", "some-platform-dir"},
								CacheTTL:     7 * (24 * time.Hour),
							},
							logger,
						).DoAndReturn(
							func(dockerfile extend.Dockerfile, toBaseImage v1.Image, withBuildOptions extend.Options, logger llog.Logger) (v1.Image, error) {
								h.AssertEq(t, dockerfile.Path, expectedDockerfileA.Path)
								h.AssertEq(t, len(dockerfile.Args), 4) // build_id, user_id, group_id
								h.AssertEq(t, dockerfile.Args[0].Name, "build_id")
								_, err := uuid.Parse(dockerfile.Args[0].Value)
								h.AssertNil(t, err)
								h.AssertEq(t, dockerfile.Args[1].Name, "user_id")
								h.AssertEq(t, dockerfile.Args[1].Value, "1234")
								h.AssertEq(t, dockerfile.Args[2].Name, "group_id")
								h.AssertEq(t, dockerfile.Args[2].Value, "5678")
								h.AssertEq(t, dockerfile.Args[3], expectedDockerfileA.Args[0])

								return someFakeImage, nil
							})
						firstConfig := &v1.ConfigFile{Config: v1.Config{
							User:   "1234:5678",
							Labels: map[string]string{lifecycle.RebasableLabel: fmt.Sprintf("%t", tc.firstDockerfileRebasable)},
						}}
						someFakeImage.ConfigFileReturnsOnCall(1, firstConfig, nil)

						// second dockerfile

						fakeDockerfileApplier.EXPECT().Apply(
							gomock.Any(),
							someFakeImage,
							extend.Options{
								BuildContext: "some-app-dir",
								IgnorePaths:  []string{"some-app-dir", "some-layers-dir", "some-platform-dir"},
								CacheTTL:     7 * (24 * time.Hour),
							},
							logger,
						).DoAndReturn(
							func(dockerfile extend.Dockerfile, toBaseImage v1.Image, withBuildOptions extend.Options, logger llog.Logger) (v1.Image, error) {
								h.AssertEq(t, dockerfile.Path, expectedDockerfileB.Path)
								h.AssertEq(t, len(dockerfile.Args), 4) // build_id, user_id, group_id
								h.AssertEq(t, dockerfile.Args[0].Name, "build_id")
								_, err := uuid.Parse(dockerfile.Args[0].Value)
								h.AssertNil(t, err)
								h.AssertEq(t, dockerfile.Args[1].Name, "user_id")
								h.AssertEq(t, dockerfile.Args[1].Value, "1234")
								h.AssertEq(t, dockerfile.Args[2].Name, "group_id")
								h.AssertEq(t, dockerfile.Args[2].Value, "5678")
								h.AssertEq(t, dockerfile.Args[3], expectedDockerfileB.Args[0])

								return someFakeImage, nil
							})
						secondConfig := &v1.ConfigFile{Config: v1.Config{
							User:   "1234:5678",
							Labels: map[string]string{lifecycle.RebasableLabel: fmt.Sprintf("%t", tc.secondDockerfileRebasable)},
						}}
						someFakeImage.ConfigFileReturnsOnCall(2, secondConfig, nil)

						// set label

						someFakeImage.ConfigFileReturnsOnCall(3, secondConfig, nil)
						someFakeImage.ConfigFileReturnsOnCall(4, secondConfig, nil)

						// save selective

						imageHash := v1.Hash{Algorithm: "sha256", Hex: "some-image-hex"}
						someFakeImage.DigestReturns(imageHash, nil)
						someFakeImage.ConfigNameReturns(v1.Hash{Algorithm: "sha256", Hex: "some-config-hex"}, nil)

						fakeDockerfileApplier.EXPECT().Cleanup().Return(nil)

						h.AssertNil(t, extender.Extend("run", logger))
						outputImagePath := filepath.Join(extendedDir, "run", tc.expectedImageSHA)
						h.AssertPathExists(t, outputImagePath)
						fis, err := os.ReadDir(outputImagePath)
						h.AssertNil(t, err)
						h.AssertEq(t, len(fis), 3)
					})
				})
			}
		})
	})
}
