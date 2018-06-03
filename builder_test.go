package lifecycle_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/sclevine/lifecycle"
	"github.com/sclevine/lifecycle/testmock"
)

func TestBuilder(t *testing.T) {
	spec.Run(t, "#Build", testBuild, spec.Report(report.Terminal{}))
}

//go:generate mockgen -package mocks -destination testmock/env.go github.com/sclevine/lifecycle Env

func testBuild(t *testing.T, _ spec.G, it spec.S) {
	// TODO: break into smaller tests
	it("builds an app", func() {
		tmpDir, err := ioutil.TempDir("", "test.lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		defer os.RemoveAll(tmpDir)

		cacheDir := filepath.Join(tmpDir, "cache")
		launchDir := filepath.Join(tmpDir, "launch")
		appDir := filepath.Join(launchDir, "app")
		platformDir := filepath.Join(tmpDir, "platform")
		mkdirs(t,
			filepath.Join(cacheDir, "buildpack1-id"),
			filepath.Join(launchDir, "buildpack1-id"),

			filepath.Join(appDir, "cache-buildpack1", "cache-layer1"),
			filepath.Join(appDir, "cache-buildpack1", "cache-layer2"),
			filepath.Join(appDir, "cache-buildpack2", "cache-layer3"),
			filepath.Join(appDir, "cache-buildpack2", "cache-layer4"),

			filepath.Join(appDir, "launch-buildpack1", "launch-layer1"),
			filepath.Join(appDir, "launch-buildpack2", "launch-layer2"),

			filepath.Join(platformDir, "env"),
		)
		mkfiles(t, "some-data",
			filepath.Join(platformDir, "env", "SOME_VAR"),
		)

		mockCtrl := gomock.NewController(t)
		defer mockCtrl.Finish()
		env := testmock.NewMockEnv(mockCtrl)

		env.EXPECT().List().Return([]string{"ID=1"})
		env.EXPECT().List().Return([]string{"ID=2"})
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

		stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
		builder := lifecycle.Builder{
			PlatformDir: platformDir,
			Buildpacks: lifecycle.BuildpackGroup{
				{ID: "buildpack1-id", Name: "buildpack1-name", Dir: filepath.Join("testdata", "buildpack")},
				{ID: "buildpack2-id", Name: "buildpack2-name", Dir: filepath.Join("testdata", "buildpack")},
			},
			Out: io.MultiWriter(stdout, it.Out()), Err: io.MultiWriter(stderr, it.Out()),
		}
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
		if stdout.String() != "STDOUT1\nSTDOUT2\n" {
			t.Fatalf("Unexpected: %s", stdout)
		}
		if stderr.String() != "STDERR1\nSTDERR2\n" {
			t.Fatalf("Unexpected: %s", stderr)
		}
		testExists(t,
			filepath.Join(launchDir, "buildpack1-id", "launch-layer1"),
			filepath.Join(launchDir, "buildpack2-id", "launch-layer2"),
			filepath.Join(appDir, "env-buildpack1", "SOME_VAR"),
			filepath.Join(appDir, "env-buildpack2", "SOME_VAR"),
		)
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
