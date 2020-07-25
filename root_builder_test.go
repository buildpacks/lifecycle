package lifecycle_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle"
	"github.com/buildpacks/lifecycle/testmock"
)

func TestRootBuilder(t *testing.T) {
	spec.Run(t, "RootBuilder", testRootBuilder, spec.Report(report.Terminal{}))
}

//go:generate mockgen -package testmock -destination testmock/env.go github.com/buildpacks/lifecycle RootBuildEnv

func testRootBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		builder        *lifecycle.RootBuilder
		mockCtrl       *gomock.Controller
		env            *testmock.MockBuildEnv
		stdout, stderr *bytes.Buffer
		tmpDir         string
		platformDir    string
		layersDir      string
		rootDir        string
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		env = testmock.NewMockBuildEnv(mockCtrl)

		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
		rootDir = filepath.Join(tmpDir, "root")
		layersDir = filepath.Join(rootDir, "layers")
		platformDir = filepath.Join(rootDir, "platform")
		mkdir(t, rootDir, layersDir, filepath.Join(platformDir, "env"))

		outLog := log.New(io.MultiWriter(stdout, it.Out()), "", 0)
		errLog := log.New(io.MultiWriter(stderr, it.Out()), "", 0)

		buildpacksDir := filepath.Join("testdata", "by-id")

		builder = &lifecycle.RootBuilder{
			PlatformDir:   platformDir,
			LayersDir:     layersDir,
			BuildpacksDir: buildpacksDir,
			Env:           env,
			Group: lifecycle.BuildpackGroup{
				Group: []lifecycle.Buildpack{
					{ID: "A", Version: "v1"},
					{ID: "B", Version: "v2"},
				},
			},
			Out: outLog,
			Err: errLog,
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
		mockCtrl.Finish()
	})

	when("#Build", func() {
		when("building succeeds", func() {
			it.Before(func() {
				env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Av1"), nil)
				env.EXPECT().WithPlatform(platformDir).Return(append(os.Environ(), "TEST_ENV=Bv2"), nil)
			})

			it("should ensure something", func() {
				if _, err := builder.Build(); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				testExists(t,
					filepath.Join(layersDir, "A.tgz"),
					filepath.Join(layersDir, "B.tgz"),
				)
			})
		})
	})
}
