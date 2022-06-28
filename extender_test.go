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
			extenderFactory *lifecycle.ExtenderFactory
			fakeAPIVerifier *testmock.MockBuildpackAPIVerifier
			fakeDirStore    *testmock.MockDirStore
			logger          *log.Logger
			mockController  *gomock.Controller
		)

		it.Before(func() {
			mockController = gomock.NewController(t)
			fakeAPIVerifier = testmock.NewMockBuildpackAPIVerifier(mockController)
			fakeDirStore = testmock.NewMockDirStore(mockController)
			logger = &log.Logger{Handler: &discard.Handler{}}

			extenderFactory = lifecycle.NewExtenderFactory(
				fakeAPIVerifier,
				fakeDirStore,
			)
		})

		it.After(func() {
			mockController.Finish()
		})

		it("configures the extender", func() {
			fakeAPIVerifier.EXPECT().VerifyBuildpackAPI(buildpack.KindExtension, "A@v1", "0.9", logger)

			extender, err := extenderFactory.NewExtender(
				[]buildpack.GroupElement{
					{ID: "A", Version: "v1", API: "0.9"},
				},
				"some-generated-dir",
				logger,
			)
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
		extender     *lifecycle.Extender
		mockCtrl     *gomock.Controller
		generatedDir string

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
		extender = &lifecycle.Extender{
			Extensions:   providedExtensions,
			GeneratedDir: generatedDir,
			Logger:       &log.Logger{Handler: logHandler},
		}
	})

	it.After(func() {
		os.RemoveAll(generatedDir)
		mockCtrl.Finish()
	})

	when(".LastRunImage", func() {
		it("determines the last run image from the provided extensions", func() {
			h.Mkdir(t, filepath.Join(generatedDir, "run", "A"))
			h.Mkfile(t, "FROM some-run-image", filepath.Join(generatedDir, "run", "A", "Dockerfile"))
			h.Mkdir(t, filepath.Join(generatedDir, "run", "B"))
			h.Mkfile(t, "FROM some-other-run-image", filepath.Join(generatedDir, "run", "B", "Dockerfile"))

			runImage, err := extender.LastRunImage()
			h.AssertNil(t, err)

			h.AssertEq(t, runImage, "some-other-run-image")
		})
	})
}
