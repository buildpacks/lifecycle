package lifecycle_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apex/log"
	"github.com/apex/log/handlers/discard"
	"github.com/apex/log/handlers/memory"
	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/api"
	"github.com/buildpacks/lifecycle/buildpack"
	"github.com/buildpacks/lifecycle/internal/extend"
	h "github.com/buildpacks/lifecycle/testhelpers"
	"github.com/buildpacks/lifecycle/testmock"
)

func TestExtender(t *testing.T) {
	spec.Run(t, "unit-new-extender", testExtenderFactory, spec.Report(report.Terminal{}))
	spec.Run(t, "unit-extender", testExtender, spec.Report(report.Terminal{}))
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
		)

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
			extender, err := extenderFactory.NewExtender(
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

			h.AssertEq(t, extender.AppDir, "some-app-dir")
			h.AssertEq(t, extender.GeneratedDir, "some-generated-dir")
			h.AssertEq(t, extender.LayersDir, "some-layers-dir")
			h.AssertEq(t, extender.PlatformDir, "some-platform-dir")
			h.AssertEq(t, extender.CacheTTL, 7*(24*time.Hour))
			h.AssertEq(t, extender.Extensions, []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.9", Extension: true},
			})
			h.AssertEq(t, extender.Logger, logger)
		})
	})
}

func testExtender(t *testing.T, when spec.G, it spec.S) {
	var (
		extender              *lifecycle.Extender
		fakeDockerfileApplier *testmock.MockDockerfileApplier
		mockCtrl              *gomock.Controller
		generatedDir          string

		logHandler = memory.New()
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)

		var err error
		generatedDir, err = ioutil.TempDir("", "lifecycle")
		h.AssertNil(t, err)

		providedExtensions := []buildpack.GroupElement{
			{ID: "A", Version: "v1", API: api.Buildpack.Latest().String(), Homepage: "A Homepage"},
			{ID: "B", Version: "v2", API: api.Buildpack.Latest().String()},
		}
		fakeDockerfileApplier = testmock.NewMockDockerfileApplier(mockCtrl)
		extender = &lifecycle.Extender{
			AppDir:            "some-app-dir",
			GeneratedDir:      generatedDir,
			LayersDir:         "some-layers-dir",
			PlatformDir:       "some-platform-dir",
			CacheTTL:          7 * (24 * time.Hour),
			DockerfileApplier: fakeDockerfileApplier,
			Extensions:        providedExtensions,
			Logger:            &log.Logger{Handler: logHandler},
		}
	})

	it.After(func() {
		os.RemoveAll(generatedDir)
		mockCtrl.Finish()
	})

	when(".ExtendBuild", func() {
		it("applies the provided Dockerfiles to the build image", func() {
			h.Mkdir(t, filepath.Join(generatedDir, "build", "B"))
			h.Mkfile(t, "some build.Dockerfile content", filepath.Join(generatedDir, "build", "B", "Dockerfile"))
			h.Mkfile(t, "[[build.args]]\nname=\"arg1\"\nvalue=\"value1\"", filepath.Join(generatedDir, "build", "B", "extend-config.toml"))

			expectedDockerfiles := []extend.Dockerfile{{
				Path: filepath.Join(generatedDir, "build", "B", "Dockerfile"),
				Args: []extend.Arg{{Name: "arg1", Value: "value1"}},
			}}
			fakeDockerfileApplier.EXPECT().Apply("some-app-dir", "some-image-ref", expectedDockerfiles, gomock.Any())

			h.AssertNil(t, extender.ExtendBuild("some-image-ref"))
		})

		when("Dockerfile is provided without config", func() {
			it("applies without error", func() {
				h.Mkdir(t, filepath.Join(generatedDir, "build", "B"))
				h.Mkfile(t, "some build.Dockerfile content", filepath.Join(generatedDir, "build", "B", "Dockerfile"))

				var empty []extend.Arg
				expectedDockerfiles := []extend.Dockerfile{{
					Path: filepath.Join(generatedDir, "build", "B", "Dockerfile"),
					Args: empty,
				}}
				fakeDockerfileApplier.EXPECT().Apply("some-app-dir", "some-image-ref", expectedDockerfiles, gomock.Any())

				h.AssertNil(t, extender.ExtendBuild("some-image-ref"))
			})
		})

		when("options", func() {
			when(":ignorepaths", func() {
				it("has <workspace>, <layers>, and <platform>", func() {
					fakeDockerfileApplier.EXPECT().Apply(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do( // FIXME: this test (and others that use `Do` or `DoAndReturn`) could be made simpler using a custom matcher
						func(_ string, _ string, _ []extend.Dockerfile, options extend.Options) error {
							h.AssertEq(t, options.IgnorePaths, []string{"some-app-dir", "some-layers-dir", "some-platform-dir"})
							return nil
						})

					h.AssertNil(t, extender.ExtendBuild("some-image-ref"))
				})
			})

			when(":cacheTTL", func() {
				it("passes it to the extend options", func() {
					fakeDockerfileApplier.EXPECT().Apply(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Do(
						func(_ string, _ string, _ []extend.Dockerfile, options extend.Options) error {
							h.AssertEq(t, options.CacheTTL, 7*(24*time.Hour))
							return nil
						})

					h.AssertNil(t, extender.ExtendBuild("some-image-ref"))
				})
			})
		})
	})
}
