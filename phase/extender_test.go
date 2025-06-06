package phase_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
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

	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/extend"
	llog "github.com/buildpacks/lifecycle/log"
	"github.com/buildpacks/lifecycle/phase"
	"github.com/buildpacks/lifecycle/phase/testmock"
	"github.com/buildpacks/lifecycle/platform"
	"github.com/buildpacks/lifecycle/platform/files"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestExtender(t *testing.T) {
	spec.Run(t, "unit-new-extender", testExtenderFactory, spec.Report(report.Terminal{}))
	spec.Run(t, "unit-extender", testExtender, spec.Sequential(), spec.Report(report.Terminal{}))
}

func testExtenderFactory(t *testing.T, when spec.G, it spec.S) {
	when("#NewExtender", func() {
		var (
			mockController    *gomock.Controller
			extenderFactory   *phase.HermeticFactory
			fakeAPIVerifier   *testmock.MockBuildpackAPIVerifier
			fakeConfigHandler *testmock.MockConfigHandler
			fakeDirStore      *testmock.MockDirStore
			logger            *log.Logger
			analyzedMD        = files.Analyzed{
				BuildImage: &files.ImageIdentifier{Reference: "some-build-image-ref"},
				RunImage:   &files.RunImage{Reference: "some-run-image-ref"},
			}
			extender *phase.Extender
			kind     = "build"
		)

		createExtender := func() {
			fakeConfigHandler.EXPECT().ReadAnalyzed("some-analyzed-path", logger).Return(
				analyzedMD, nil,
			).AnyTimes()
			fakeConfigHandler.EXPECT().ReadGroup("some-group-path").Return(
				buildpack.Group{GroupExtensions: []buildpack.GroupElement{{ID: "A", Version: "v1", API: "0.9", Extension: true}}}, nil,
			).AnyTimes()
			fakeDirStore.EXPECT().LookupExt("A", "v1").Return(&buildpack.ExtDescriptor{
				WithAPI: "0.9",
				Extension: buildpack.ExtInfo{
					BaseInfo: buildpack.BaseInfo{
						ID:      "A",
						Version: "v1",
					},
				},
			}, nil).AnyTimes()
			fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindExtension, "A@v1", "0.9", logger).AnyTimes()
			fakeDockerfileApplier := testmock.NewMockDockerfileApplier(mockController)

			var err error
			extender, err = extenderFactory.NewExtender(platform.LifecycleInputs{
				AnalyzedPath:   "some-analyzed-path",
				AppDir:         "some-app-dir",
				ExtendedDir:    "some-extended-dir",
				GeneratedDir:   "some-generated-dir",
				GroupPath:      "some-group-path",
				LayersDir:      "some-layers-dir",
				PlatformDir:    "some-platform-dir",
				KanikoCacheTTL: 7 * (24 * time.Hour),
				ExtendKind:     kind,
			}, fakeDockerfileApplier, logger)
			h.AssertNil(t, err)
		}

		it.Before(func() {
			mockController = gomock.NewController(t)
			fakeAPIVerifier = testmock.NewMockBuildpackAPIVerifier(mockController)
			fakeConfigHandler = testmock.NewMockConfigHandler(mockController)
			fakeDirStore = testmock.NewMockDirStore(mockController)
			extenderFactory = phase.NewHermeticFactory(api.Platform.Latest(), fakeAPIVerifier, fakeConfigHandler, nil)

			logger = &log.Logger{Handler: &discard.Handler{}}
		})

		it.After(func() {
			mockController.Finish()
		})

		it("configures the extender", func() {
			createExtender()

			h.AssertEq(t, extender.AppDir, "some-app-dir")
			h.AssertEq(t, extender.ExtendedDir, "some-extended-dir")
			h.AssertEq(t, extender.GeneratedDir, "some-generated-dir")
			h.AssertEq(t, extender.ImageRef, "some-build-image-ref")
			h.AssertEq(t, extender.LayersDir, "some-layers-dir")
			h.AssertEq(t, extender.PlatformDir, "some-platform-dir")
			h.AssertEq(t, extender.CacheTTL, 7*(24*time.Hour))
			h.AssertEq(t, extender.Extensions, []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.9", Extension: true},
			})
		})

		when("build image is nil", func() {
			it("does not panic", func() {
				analyzedMD = files.Analyzed{}
				createExtender()
			})
		})

		when("kind", func() {
			when("build", func() {
				kind = "build"

				it("configures the extender with the build image", func() {
					createExtender()
					h.AssertEq(t, extender.ImageRef, "some-build-image-ref")
				})
			})

			when("run", func() {
				kind = "run"

				it("configures the extender with the run image", func() {
					createExtender()
					h.AssertEq(t, extender.ImageRef, "some-run-image-ref")
				})
			})
		})
	})
}

func testExtender(t *testing.T, when spec.G, it spec.S) {
	when("Extend", func() {
		var (
			extender              *phase.Extender
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
			extender = &phase.Extender{
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

		// prepares fixtures and expected data for extensions
		// On older platforms, the extender looks for Dockerfiles in <generated>/<kind>/<extension-id>/Dockerfile.
		// On newer platforms, it looks for Dockerfiles in <generated>/<extension-id>/kind.Dockerfile, and context directories are allowed.
		prepareDockerfile := func(id, kind, contextDir string) extend.Dockerfile {
			basePath := filepath.Join(generatedDir, kind, id)
			dockerfilePath := filepath.Join(basePath, "Dockerfile")

			if extender.PlatformAPI.AtLeast("0.13") {
				basePath = filepath.Join(generatedDir, id)
				dockerfilePath = filepath.Join(basePath, fmt.Sprintf("%s.Dockerfile", kind))

				switch contextDir {
				case "specific":
					h.Mkdir(t, filepath.Join(basePath, fmt.Sprintf("context.%s", kind)))
				case "shared":
					h.Mkdir(t, filepath.Join(basePath, "context"))
				case "app":
					// nothing to do
				default:
					t.Fail()
				}
			} else {
				h.AssertEq(t, contextDir, "app")
			}

			expectedDockerfile := extend.Dockerfile{
				Path: dockerfilePath,
				Args: []extend.Arg{{Name: fmt.Sprintf("arg%s", id), Value: fmt.Sprintf("value%s", id)}},
			}
			h.Mkdir(t, basePath)
			h.Mkfile(t, "some dockerfile content", dockerfilePath)

			buf := new(bytes.Buffer)
			var data extend.Config
			switch kind {
			case "run":
				data = extend.Config{Run: extend.BuildConfig{Args: expectedDockerfile.Args}}
			case "build":
				data = extend.Config{Build: extend.BuildConfig{Args: expectedDockerfile.Args}}
			default:
				t.Fail()
			}
			h.AssertNil(t, toml.NewEncoder(buf).Encode(data))
			h.Mkfile(t, buf.String(), filepath.Join(basePath, "extend-config.toml"))
			return expectedDockerfile
		}

		for _, platformAPI := range []*api.Version{
			api.MustParse("0.12"),
			api.MustParse("0.13"),
		} {
			when(fmt.Sprintf("using the platform API %s", platformAPI), func() {
				it.Before(func() {
					extender.PlatformAPI = platformAPI
				})

				when("build base image", func() {
					it("applies the Dockerfile with the expected args and opts", func() {
						var expectedDockerfileA, expectedDockerfileB extend.Dockerfile
						var expectedBuildContextA, expectedBuildContextB string

						if platformAPI.AtLeast("0.13") {
							expectedDockerfileA = prepareDockerfile("A", "build", "specific")
							expectedBuildContextA = filepath.Join(generatedDir, "A", "context.build")
							expectedDockerfileB = prepareDockerfile("B", "build", "shared")
							expectedBuildContextB = filepath.Join(generatedDir, "B", "context")
						} else {
							expectedDockerfileA = prepareDockerfile("A", "build", "app")
							expectedBuildContextA = "some-app-dir"
							expectedDockerfileB = prepareDockerfile("B", "build", "app")
							expectedBuildContextB = "some-app-dir"
						}

						fakeDockerfileApplier.EXPECT().ImageFor(extender.ImageRef).Return(someFakeImage, nil)
						someFakeImage.ManifestReturns(&v1.Manifest{Layers: []v1.Descriptor{}}, nil)

						// first dockerfile

						firstConfig := &v1.ConfigFile{Config: v1.Config{
							User: "0:5678",
						}}
						someFakeImage.ConfigFileReturnsOnCall(0, firstConfig, nil)
						fakeDockerfileApplier.EXPECT().Apply(
							gomock.Any(),
							gomock.Any(), // we mutate the provided image so we can't expect the fake image
							extend.Options{
								BuildContext: expectedBuildContextA,
								IgnorePaths:  []string{"some-app-dir", "some-layers-dir", "some-platform-dir"},
								CacheTTL:     7 * (24 * time.Hour),
							},
							logger,
						).DoAndReturn(
							func(dockerfile extend.Dockerfile, _ v1.Image, _ extend.Options, _ llog.Logger) (v1.Image, error) {
								h.AssertEq(t, dockerfile.Path, expectedDockerfileA.Path)
								h.AssertEq(t, len(dockerfile.Args), 4)
								h.AssertEq(t, dockerfile.Args[0].Name, "build_id")
								_, err := uuid.Parse(dockerfile.Args[0].Value)
								h.AssertNil(t, err)
								h.AssertEq(t, dockerfile.Args[1].Name, "user_id")
								h.AssertEq(t, dockerfile.Args[1].Value, "0")
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

						someFakeImage.ConfigFileReturnsOnCall(2, secondConfig, nil)
						fakeDockerfileApplier.EXPECT().Apply(
							gomock.Any(),
							someFakeImage,
							extend.Options{
								BuildContext: expectedBuildContextB,
								IgnorePaths:  []string{"some-app-dir", "some-layers-dir", "some-platform-dir"},
								CacheTTL:     7 * (24 * time.Hour),
							},
							logger,
						).DoAndReturn(
							func(dockerfile extend.Dockerfile, _ v1.Image, _ extend.Options, _ llog.Logger) (v1.Image, error) {
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
						when := when
						desc := func(b bool) string {
							if b {
								return "rebasable"
							}
							return "not rebasable"
						}
						when(fmt.Sprintf("first Dockerfile is %s, second Dockerfile is %s", desc(tc.firstDockerfileRebasable), desc(tc.secondDockerfileRebasable)), func() {
							it("applies the Dockerfile with the expected args and opts", func() {
								expectedDockerfileA := prepareDockerfile("A", "run", "app")
								expectedDockerfileB := prepareDockerfile("B", "run", "app")

								fakeDockerfileApplier.EXPECT().ImageFor(extender.ImageRef).Return(someFakeImage, nil)
								someFakeImage.ManifestReturns(&v1.Manifest{Layers: []v1.Descriptor{}}, nil)
								someFakeImage.ConfigFileReturnsOnCall(0, &v1.ConfigFile{Config: v1.Config{
									User: "1234:5678",
								}}, nil)

								// first dockerfile

								fakeDockerfileApplier.EXPECT().Apply(
									gomock.Any(),
									gomock.Any(), // we mutate the provided image so we can't expect the fake image
									extend.Options{
										BuildContext: "some-app-dir",
										IgnorePaths:  []string{"some-app-dir", "some-layers-dir", "some-platform-dir"},
										CacheTTL:     7 * (24 * time.Hour),
									},
									logger,
								).DoAndReturn(
									func(dockerfile extend.Dockerfile, _ v1.Image, _ extend.Options, _ llog.Logger) (v1.Image, error) {
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
									Labels: map[string]string{phase.RebasableLabel: fmt.Sprintf("%t", tc.firstDockerfileRebasable)},
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
									func(dockerfile extend.Dockerfile, _ v1.Image, _ extend.Options, _ llog.Logger) (v1.Image, error) {
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
									Labels: map[string]string{phase.RebasableLabel: fmt.Sprintf("%t", tc.secondDockerfileRebasable)},
								}}
								someFakeImage.ConfigFileReturnsOnCall(2, secondConfig, nil)

								// set label

								someFakeImage.ConfigFileReturnsOnCall(3, secondConfig, nil)
								someFakeImage.ConfigFileReturnsOnCall(4, secondConfig, nil)

								// save without base layers

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

					it("errors if the last extension leaves the user as root", func() {
						expectedDockerfileA := prepareDockerfile("A", "run", "app")

						fakeDockerfileApplier.EXPECT().ImageFor(extender.ImageRef).Return(someFakeImage, nil)
						firstConfig := &v1.ConfigFile{Config: v1.Config{
							User: "0:5678",
						}}
						someFakeImage.ConfigFileReturns(firstConfig, nil)
						someFakeImage.ManifestReturns(&v1.Manifest{Layers: []v1.Descriptor{}}, nil)

						fakeDockerfileApplier.EXPECT().Apply(
							gomock.Any(),
							gomock.Any(),
							gomock.Any(),
							logger,
						).DoAndReturn(
							func(dockerfile extend.Dockerfile, _ v1.Image, _ extend.Options, _ llog.Logger) (v1.Image, error) {
								h.AssertEq(t, dockerfile.Path, expectedDockerfileA.Path)
								h.AssertEq(t, len(dockerfile.Args), 4)
								h.AssertEq(t, dockerfile.Args[0].Name, "build_id")
								_, err := uuid.Parse(dockerfile.Args[0].Value)
								h.AssertNil(t, err)
								h.AssertEq(t, dockerfile.Args[1].Name, "user_id")
								h.AssertEq(t, dockerfile.Args[1].Value, "0")
								h.AssertEq(t, dockerfile.Args[2].Name, "group_id")
								h.AssertEq(t, dockerfile.Args[2].Value, "5678")
								h.AssertEq(t, dockerfile.Args[3], expectedDockerfileA.Args[0])

								return someFakeImage, nil
							})

						err := extender.Extend("run", logger)
						h.AssertError(t, err, "extending run image: the final user ID is 0 (root); please add another extension that resets the user to non-root")
					})
				})
			})
		}
	})
}
