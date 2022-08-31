package lifecycle_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

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
			extenderFactory   *lifecycle.ExtenderFactory
			fakeAPIVerifier   *testmock.MockBuildpackAPIVerifier
			fakeConfigHandler *testmock.MockConfigHandler
			logger            *log.Logger
			mockController    *gomock.Controller
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			fakeAPIVerifier = testmock.NewMockBuildpackAPIVerifier(mockController)
			fakeConfigHandler = testmock.NewMockConfigHandler(mockController)
			logger = &log.Logger{Handler: &discard.Handler{}}

			extenderFactory = lifecycle.NewExtenderFactory(fakeAPIVerifier, fakeConfigHandler)
		})

		it.After(func() {
			mockController.Finish()
		})

		it("configures the extender", func() {
			fakeConfigHandler.EXPECT().ReadGroup("some-group-path").Return(
				[]buildpack.GroupElement{},
				[]buildpack.GroupElement{{ID: "A", Version: "v1", API: "0.9"}},
				nil,
			)
			fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindExtension, "A@v1", "0.9", logger)

			extender, err := extenderFactory.NewExtender(nil, "some-group-path", "some-generated-dir", logger)
			h.AssertNil(t, err)

			h.AssertEq(t, extender.Extensions, []buildpack.GroupElement{
				{ID: "A", Version: "v1", API: "0.9"},
			})
			h.AssertEq(t, extender.GeneratedDir, "some-generated-dir")
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
			Extensions:        providedExtensions,
			GeneratedDir:      generatedDir,
			Logger:            &log.Logger{Handler: logHandler},
			DockerfileApplier: fakeDockerfileApplier,
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
			fakeDockerfileApplier.EXPECT().Apply(expectedDockerfiles, "some-image-ref", gomock.Any())

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
				fakeDockerfileApplier.EXPECT().Apply(expectedDockerfiles, "some-image-ref", gomock.Any())

				h.AssertNil(t, extender.ExtendBuild("some-image-ref"))
			})
		})
	})
}
