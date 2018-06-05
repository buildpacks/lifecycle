package lifecycle_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/sclevine/lifecycle"
	"github.com/sclevine/lifecycle/testmock"
)

func TestBuilder(t *testing.T) {
	spec.Run(t, "Builder", testBuilder, spec.Report(report.Terminal{}))
}

//go:generate mockgen -package mocks -destination testmock/env.go github.com/sclevine/lifecycle Env

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		builder             *lifecycle.Builder
		mockCtrl            *gomock.Controller
		env                 *testmock.MockEnv
		stdout, stderr      *bytes.Buffer
		tmpDir, platformDir string
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		env = testmock.NewMockEnv(mockCtrl)

		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle")
		if err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		platformDir = filepath.Join(tmpDir, "platform")
		mkdir(t, filepath.Join(platformDir, "env"))

		stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
		buildpackDir := filepath.Join("testdata", "buildpack")
		builder = &lifecycle.Builder{
			PlatformDir: platformDir,
			Buildpacks: lifecycle.BuildpackGroup{
				{ID: "buildpack1-id", Dir: buildpackDir},
				{ID: "buildpack2-id", Dir: buildpackDir},
			},
			Out: io.MultiWriter(stdout, it.Out()),
			Err: io.MultiWriter(stderr, it.Out()),
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
			mkdir(t, cacheDir, launchDir, appDir)
		})

		when("building succeeds", func() {
			it.Before(func() {
				env.EXPECT().List().Return([]string{"ID=1"})
				env.EXPECT().List().Return([]string{"ID=2"})
			})

			it("should ensure each cache dir exists and process it", func() {
				mkdir(t,
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
				if _, err := builder.Build(appDir, cacheDir, launchDir, env); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
			})

			it("should ensure each launch dir exists and process it", func() {
				mkdir(t,
					filepath.Join(launchDir, "buildpack1-id"),

					filepath.Join(appDir, "launch-buildpack1", "launch-layer1"),
					filepath.Join(appDir, "launch-buildpack2", "launch-layer2"),
				)
				if _, err := builder.Build(appDir, cacheDir, launchDir, env); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				testExists(t,
					filepath.Join(launchDir, "buildpack1-id", "launch-layer1"),
					filepath.Join(launchDir, "buildpack2-id", "launch-layer2"),
				)
			})

			it("should return launch metadata", func() {
				metadata, err := builder.Build(appDir, cacheDir, launchDir, env)
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
				mkfile(t, "some-data",
					filepath.Join(platformDir, "env", "SOME_VAR"),
				)
				if _, err := builder.Build(appDir, cacheDir, launchDir, env); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				testExists(t,
					filepath.Join(appDir, "env-buildpack1", "SOME_VAR"),
					filepath.Join(appDir, "env-buildpack2", "SOME_VAR"),
				)
			})

			it("should connect stdout and stdin to the terminal", func() {
				if _, err := builder.Build(appDir, cacheDir, launchDir, env); err != nil {
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

		when("building fails", func() {
			it("should error when launch directories cannot be created", func() {
				mkfile(t, "some-data", filepath.Join(launchDir, "buildpack1-id"))
				_, err := builder.Build(appDir, cacheDir, launchDir, env)
				if _, ok := err.(*os.PathError); !ok {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			it("should error when cache directories cannot be created", func() {
				mkfile(t, "some-data", filepath.Join(cacheDir, "buildpack1-id"))
				_, err := builder.Build(appDir, cacheDir, launchDir, env)
				if _, ok := err.(*os.PathError); !ok {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			it("should error when the command fails", func() {
				env.EXPECT().List().Return([]string{"ID=1"})
				if err := os.RemoveAll(platformDir); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				_, err := builder.Build(appDir, cacheDir, launchDir, env)
				if _, ok := err.(*exec.ExitError); !ok {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			var appendErr error
			each(it, "should error when modifying the env fails", []func(){
				func() {
					env.EXPECT().AppendDirs(gomock.Any()).Return(appendErr)
				},
				func() {
					env.EXPECT().AppendDirs(gomock.Any()).Return(nil)
					env.EXPECT().SetEnvDir(gomock.Any()).Return(appendErr)
				},
				func() {
					env.EXPECT().AppendDirs(gomock.Any()).Return(nil)
					env.EXPECT().SetEnvDir(gomock.Any()).Return(nil)
					env.EXPECT().AddEnvDir(gomock.Any()).Return(appendErr)
				},
			}, func(mock func()) {
				appendErr = errors.New("some error")
				env.EXPECT().List().Return([]string{"ID=1"})
				mkdir(t, filepath.Join(appDir, "cache-buildpack1", "cache-layer1"))
				mock()
				if _, err := builder.Build(appDir, cacheDir, launchDir, env); err != appendErr {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			it("should error when launch.toml is not writable", func() {
				env.EXPECT().List().Return([]string{"ID=1"})
				mkdir(t, filepath.Join(platformDir, "launch.toml"))
				if _, err := builder.Build(appDir, cacheDir, launchDir, env); err == nil {
					t.Fatal("Expected error")
				}
			})
		})
	})

	when("#Develop", func() {
		var (
			appDir   string
			cacheDir string
		)

		it.Before(func() {
			cacheDir = filepath.Join(tmpDir, "cache")
			appDir = filepath.Join(tmpDir, "app")
			mkdir(t, cacheDir, appDir)
		})

		when("developing succeeds", func() {
			it.Before(func() {
				env.EXPECT().List().Return([]string{"ID=1"})
				env.EXPECT().List().Return([]string{"ID=2"})
			})

			it("should ensure each cache dir exists and process it", func() {
				mkdir(t,
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
				if _, err := builder.Develop(appDir, cacheDir, env); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
			})

			it("should return development metadata", func() {
				metadata, err := builder.Develop(appDir, cacheDir, env)
				if err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				if !reflect.DeepEqual(metadata, &lifecycle.DevelopMetadata{
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
				mkfile(t, "some-data",
					filepath.Join(platformDir, "env", "SOME_VAR"),
				)
				if _, err := builder.Develop(appDir, cacheDir, env); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				testExists(t,
					filepath.Join(appDir, "env-buildpack1", "SOME_VAR"),
					filepath.Join(appDir, "env-buildpack2", "SOME_VAR"),
				)
			})

			it("should connect stdout and stdin to the terminal", func() {
				if _, err := builder.Develop(appDir, cacheDir, env); err != nil {
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

		when("developing fails", func() {
			it("should error when cache directories cannot be created", func() {
				mkfile(t, "some-data", filepath.Join(cacheDir, "buildpack1-id"))
				_, err := builder.Develop(appDir, cacheDir, env)
				if _, ok := err.(*os.PathError); !ok {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			it("should error when the command fails", func() {
				env.EXPECT().List().Return([]string{"ID=1"})
				if err := os.RemoveAll(platformDir); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				_, err := builder.Develop(appDir, cacheDir, env)
				if _, ok := err.(*exec.ExitError); !ok {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			var appendErr error
			each(it, "should error when modifying the env fails", []func(){
				func() {
					env.EXPECT().AppendDirs(gomock.Any()).Return(appendErr)
				},
				func() {
					env.EXPECT().AppendDirs(gomock.Any()).Return(nil)
					env.EXPECT().SetEnvDir(gomock.Any()).Return(appendErr)
				},
				func() {
					env.EXPECT().AppendDirs(gomock.Any()).Return(nil)
					env.EXPECT().SetEnvDir(gomock.Any()).Return(nil)
					env.EXPECT().AddEnvDir(gomock.Any()).Return(appendErr)
				},
			}, func(mock func()) {
				appendErr = errors.New("some error")
				env.EXPECT().List().Return([]string{"ID=1"})
				mkdir(t, filepath.Join(appDir, "cache-buildpack1", "cache-layer1"))
				mock()
				if _, err := builder.Develop(appDir, cacheDir, env); err != appendErr {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			it("should error when develop.toml is not writable", func() {
				env.EXPECT().List().Return([]string{"ID=1"})
				mkdir(t, filepath.Join(platformDir, "develop.toml"))
				if _, err := builder.Develop(appDir, cacheDir, env); err == nil {
					t.Fatal("Expected error")
				}
			})
		})
	})
}

func mkdir(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0777); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func mkfile(t *testing.T, data string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := ioutil.WriteFile(p, []byte(data), 0777); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("Error: %s\n", err)
	}
	return strings.TrimSpace(string(b))
}

func testExists(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func each(it spec.S, text string, ops []func(), test func(func())) {
	for i, op := range ops {
		it(fmt.Sprintf("%s #%d", text, i), func() { test(op) })
	}
}
