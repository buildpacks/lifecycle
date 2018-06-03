package lifecycle_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"reflect"

	"github.com/sclevine/lifecycle"
	"github.com/sclevine/lifecycle/testmock"
)

func TestBuilder(t *testing.T) {
	spec.Run(t, "Builder", testBuilder, spec.Report(report.Terminal{}))
}

//go:generate mockgen -package mocks -destination testmock/env.go github.com/sclevine/lifecycle Env

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		builder        *lifecycle.Builder
		env            *testmock.MockEnv
		mockCtrl       *gomock.Controller
		tmpDir         string
		platformDir    string
		stdout, stderr *bytes.Buffer
	)

	it.Before(func() {
		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		platformDir = filepath.Join(tmpDir, "platform")
		mkdirs(t, filepath.Join(platformDir, "env"))

		mockCtrl = gomock.NewController(t)
		env = testmock.NewMockEnv(mockCtrl)
		env.EXPECT().List().Return([]string{"ID=1"})
		env.EXPECT().List().Return([]string{"ID=2"})

		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
		builder = &lifecycle.Builder{
			PlatformDir: platformDir,
			Buildpacks: lifecycle.BuildpackGroup{
				{ID: "buildpack1-id", Name: "buildpack1-name", Dir: filepath.Join("testdata", "buildpack")},
				{ID: "buildpack2-id", Name: "buildpack2-name", Dir: filepath.Join("testdata", "buildpack")},
			},
			Out: io.MultiWriter(stdout, it.Out()), Err: io.MultiWriter(stderr, it.Out()),
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
		mockCtrl.Finish()
	})

	when("#Build", func() {
		var (
			appDir    string
			launchDir string
			cacheDir  string
		)

		it.Before(func() {
			cacheDir = filepath.Join(tmpDir, "cache")
			launchDir = filepath.Join(tmpDir, "launch")
			appDir = filepath.Join(launchDir, "app")
			mkdirs(t, cacheDir, launchDir, appDir)
		})

		it("should ensure each cache dir exists and process it", func() {
			mkdirs(t,
				filepath.Join(cacheDir, "buildpack1-id"),

				filepath.Join(appDir, "cache-buildpack1", "cache-layer1"),
				filepath.Join(appDir, "cache-buildpack1", "cache-layer2"),
				filepath.Join(appDir, "cache-buildpack2", "cache-layer3"),
				filepath.Join(appDir, "cache-buildpack2", "cache-layer4"),
			)

			gomock.InOrder(
				env.EXPECT().AppendDirs(filepath.Join(cacheDir, "buildpack1-id", "cache-layer1")),
				env.EXPECT().AppendDirs(filepath.Join(cacheDir, "buildpack1-id", "cache-layer2")),
				env.EXPECT().SetEnvDir(filepath.Join(cacheDir, "buildpack1-id", "cache-layer1", "env", "set")),
				env.EXPECT().SetEnvDir(filepath.Join(cacheDir, "buildpack1-id", "cache-layer2", "env", "set")),
				env.EXPECT().AddEnvDir(filepath.Join(cacheDir, "buildpack1-id", "cache-layer1", "env", "add")),
				env.EXPECT().AddEnvDir(filepath.Join(cacheDir, "buildpack1-id", "cache-layer2", "env", "add")),

				env.EXPECT().AppendDirs(filepath.Join(cacheDir, "buildpack2-id", "cache-layer3")),
				env.EXPECT().AppendDirs(filepath.Join(cacheDir, "buildpack2-id", "cache-layer4")),
				env.EXPECT().SetEnvDir(filepath.Join(cacheDir, "buildpack2-id", "cache-layer3", "env", "set")),
				env.EXPECT().SetEnvDir(filepath.Join(cacheDir, "buildpack2-id", "cache-layer4", "env", "set")),
				env.EXPECT().AddEnvDir(filepath.Join(cacheDir, "buildpack2-id", "cache-layer3", "env", "add")),
				env.EXPECT().AddEnvDir(filepath.Join(cacheDir, "buildpack2-id", "cache-layer4", "env", "add")),
			)

			_, err := builder.Build(appDir, launchDir, cacheDir, env)
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
		})

		it("should ensure each launch dir exists and process it", func() {
			mkdirs(t,
				filepath.Join(launchDir, "buildpack1-id"),

				filepath.Join(appDir, "launch-buildpack1", "launch-layer1"),
				filepath.Join(appDir, "launch-buildpack2", "launch-layer2"),
			)

			_, err := builder.Build(appDir, launchDir, cacheDir, env)
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			testExists(t,
				filepath.Join(launchDir, "buildpack1-id", "launch-layer1"),
				filepath.Join(launchDir, "buildpack2-id", "launch-layer2"),
			)
		})

		it("should return launch metadata", func() {
			metadata, err := builder.Build(appDir, launchDir, cacheDir, env)
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			if !reflect.DeepEqual(metadata, &lifecycle.BuildMetadata{
				Processes: []lifecycle.Process{
					{Type: "override-type", Command: "process2-command"},
					{Type: "process1-type", Command: "process1-command"},
					{Type: "process2-type", Command: "process2-command"},
				},
			}) {
				t.Fatalf("Unexpected:\n%+v\n", metadata)
			}
		})

		it("should provide the platform dir", func() {
			mkfiles(t, "some-data",
				filepath.Join(platformDir, "env", "SOME_VAR"),
			)
			_, err := builder.Build(appDir, launchDir, cacheDir, env)
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			testExists(t,
				filepath.Join(appDir, "env-buildpack1", "SOME_VAR"),
				filepath.Join(appDir, "env-buildpack2", "SOME_VAR"),
			)
		})

		it("should connect stdout and stdin to the terminal", func() {
			_, err := builder.Build(appDir, launchDir, cacheDir, env)
			if err != nil {
				t.Fatalf("Error: %s\n", err)
			}
			if stdout.String() != "STDOUT1\nSTDOUT2\n" {
				t.Fatalf("Unexpected: %s", stdout)
			}
			if stderr.String() != "STDERR1\nSTDERR2\n" {
				t.Fatalf("Unexpected: %s", stderr)
			}
		})
	})
}

func mkdirs(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0777); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func mkfiles(t *testing.T, data string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := ioutil.WriteFile(p, []byte(data), 0777); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func testExists(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}
