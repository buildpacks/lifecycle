package lifecycle_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/testmock"
)

func TestBuilder(t *testing.T) {
	spec.Run(t, "Builder", testBuilder, spec.Report(report.Terminal{}))
}

//go:generate mockgen -package testmock -destination testmock/env.go github.com/buildpack/lifecycle BuildEnv

func testBuilder(t *testing.T, when spec.G, it spec.S) {
	var (
		builder        *lifecycle.Builder
		mockCtrl       *gomock.Controller
		env            *testmock.MockBuildEnv
		stdout, stderr *bytes.Buffer
		tmpDir         string
		platformDir    string
		appDir         string
		layersDir      string
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
		platformDir = filepath.Join(tmpDir, "platform")
		layersDir = filepath.Join(tmpDir, "launch")
		appDir = filepath.Join(layersDir, "app")
		mkdir(t, layersDir, appDir, filepath.Join(platformDir, "env"))
		mkfile(t, "replace = true", filepath.Join(appDir, "dep-replace"))

		outLog := log.New(io.MultiWriter(stdout, it.Out()), "", 0)
		errLog := log.New(io.MultiWriter(stderr, it.Out()), "", 0)

		buildpackDir := filepath.Join("testdata", "buildpack")
		builder = &lifecycle.Builder{
			PlatformDir: platformDir,
			LayersDir:   layersDir,
			AppDir:      appDir,
			Env:         env,
			Buildpacks: []*lifecycle.Buildpack{
				{ID: "buildpack1-id", Dir: buildpackDir},
				{ID: "buildpack2-id", Dir: buildpackDir},
			},
			Plan: lifecycle.Plan{
				"dep1":         {"v": "1"},
				"dep1-keep":    {"v": "2"},
				"dep1-replace": {"v": "3"},
				"dep2":         {"v": "4"},
				"dep2-keep":    {"v": "5"},
				"dep2-replace": {"v": "6"},
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
				env.EXPECT().List().Return([]string{"ID=1"})
				env.EXPECT().List().Return([]string{"ID=2"})
			})

			it("should ensure each buildpack's layers dir exists and process build layers", func() {
				mkdir(t,
					filepath.Join(layersDir, "buildpack1-id"),

					filepath.Join(appDir, "buildpack1", "layer1"),
					filepath.Join(appDir, "buildpack1", "layer2"),
					filepath.Join(appDir, "buildpack1", "layer3"),
					filepath.Join(appDir, "buildpack2", "layer4"),
					filepath.Join(appDir, "buildpack2", "layer5"),
					filepath.Join(appDir, "buildpack2", "layer6"),
				)
				mkfile(t, "build = true",
					filepath.Join(appDir, "buildpack1", "layer1.toml"),
					filepath.Join(appDir, "buildpack1", "layer3.toml"),
					filepath.Join(appDir, "buildpack2", "layer4.toml"),
					filepath.Join(appDir, "buildpack2", "layer6.toml"),
				)
				gomock.InOrder(
					env.EXPECT().AddRootDir(filepath.Join(layersDir, "buildpack1-id", "layer1")),
					env.EXPECT().AddRootDir(filepath.Join(layersDir, "buildpack1-id", "layer3")),
					env.EXPECT().AddEnvDir(filepath.Join(layersDir, "buildpack1-id", "layer1", "env")),
					env.EXPECT().AddEnvDir(filepath.Join(layersDir, "buildpack1-id", "layer1", "env.build")),
					env.EXPECT().AddEnvDir(filepath.Join(layersDir, "buildpack1-id", "layer3", "env")),
					env.EXPECT().AddEnvDir(filepath.Join(layersDir, "buildpack1-id", "layer3", "env.build")),

					env.EXPECT().AddRootDir(filepath.Join(layersDir, "buildpack2-id", "layer4")),
					env.EXPECT().AddRootDir(filepath.Join(layersDir, "buildpack2-id", "layer6")),
					env.EXPECT().AddEnvDir(filepath.Join(layersDir, "buildpack2-id", "layer4", "env")),
					env.EXPECT().AddEnvDir(filepath.Join(layersDir, "buildpack2-id", "layer4", "env.build")),
					env.EXPECT().AddEnvDir(filepath.Join(layersDir, "buildpack2-id", "layer6", "env")),
					env.EXPECT().AddEnvDir(filepath.Join(layersDir, "buildpack2-id", "layer6", "env.build")),
				)
				if _, err := builder.Build(); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				testExists(t,
					filepath.Join(layersDir, "buildpack1-id"),
					filepath.Join(layersDir, "buildpack2-id"),
				)
			})

			it("should return build metadata when processes are present", func() {
				metadata, err := builder.Build()
				if err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				if s := cmp.Diff(metadata, &lifecycle.BuildMetadata{
					Processes: []lifecycle.Process{
						{Type: "override-type", Command: "process2-command"},
						{Type: "process1-type", Command: "process1-command"},
						{Type: "process2-type", Command: "process2-command"},
					},
					Buildpacks: []string{"buildpack1-id", "buildpack2-id"},
					BOM: lifecycle.Plan{
						"dep1":         {"v": "1"},
						"dep1-keep":    {"v": "2"},
						"dep1-replace": {"replace": true},
						"dep2":         {"v": "4"},
						"dep2-keep":    {"v": "5"},
						"dep2-replace": {"replace": true},
					},
				}); s != "" {
					t.Fatalf("Unexpected metadata:\n%s\n", s)
				}
			})

			it("should return build metadata when processes are not present", func() {
				mkfile(t, "test", filepath.Join(appDir, "skip-processes"))
				metadata, err := builder.Build()
				if err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				if s := cmp.Diff(metadata, &lifecycle.BuildMetadata{
					Processes:  []lifecycle.Process{},
					Buildpacks: []string{"buildpack1-id", "buildpack2-id"},
					BOM: lifecycle.Plan{
						"dep1":         {"v": "1"},
						"dep1-keep":    {"v": "2"},
						"dep1-replace": {"replace": true},
						"dep2":         {"v": "4"},
						"dep2-keep":    {"v": "5"},
						"dep2-replace": {"replace": true},
					},
				}); s != "" {
					t.Fatalf("Unexpected:\n%s\n", s)
				}
			})

			it("should provide the platform dir", func() {
				mkfile(t, "some-data",
					filepath.Join(platformDir, "env", "SOME_VAR"),
				)
				if _, err := builder.Build(); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				testExists(t,
					filepath.Join(appDir, "env-buildpack1", "SOME_VAR"),
					filepath.Join(appDir, "env-buildpack2", "SOME_VAR"),
				)
			})

			it("should connect stdout and stdin to the terminal", func() {
				if _, err := builder.Build(); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				if stdout.String() != "STDOUT1\nSTDOUT2\n" {
					t.Fatalf("Unexpected: %s", stdout)
				}
				if stderr.String() != "STDERR1\nSTDERR2\n" {
					t.Fatalf("Unexpected: %s", stderr)
				}
			})

			it("should provide a subset of the build plan to each buildpack", func() {
				if _, err := builder.Build(); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				testPlan(t,
					lifecycle.Plan{
						"dep1":         {"v": "1"},
						"dep1-keep":    {"v": "2"},
						"dep1-replace": {"v": "3"},
						"dep2":         {"v": "4"},
						"dep2-keep":    {"v": "5"},
						"dep2-replace": {"v": "6"},
					},
					filepath.Join(appDir, "plan1.toml"),
				)
				testPlan(t,
					lifecycle.Plan{
						"dep1":         {"v": "1"},
						"dep2":         {"v": "4"},
						"dep2-keep":    {"v": "5"},
						"dep2-replace": {"v": "6"},
					},
					filepath.Join(appDir, "plan2.toml"),
				)
			})
		})

		when("building fails", func() {
			it("should error when layer directories cannot be created", func() {
				mkfile(t, "some-data", filepath.Join(layersDir, "buildpack1-id"))
				_, err := builder.Build()
				if _, ok := err.(*os.PathError); !ok {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			it("should error when the provided build plan is invalid", func() {
				builder.Plan = lifecycle.Plan{"bad-entry": {"f": map[int64]int64{1: 2}}}
				if _, err := builder.Build(); err == nil {
					t.Fatal("Expected error.\n")
				} else if !strings.Contains(err.Error(), "toml") {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			it("should error when any build plan entry is invalid", func() {
				env.EXPECT().List().Return([]string{"ID=1"})
				mkfile(t, "bad-key", filepath.Join(appDir, "dep-replace"))
				if _, err := builder.Build(); err == nil {
					t.Fatal("Expected error.\n")
				} else if !strings.Contains(err.Error(), "key") {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			it("should error when the command fails", func() {
				env.EXPECT().List().Return([]string{"ID=1"})
				if err := os.RemoveAll(platformDir); err != nil {
					t.Fatalf("Error: %s\n", err)
				}
				_, err := builder.Build()
				if _, ok := err.(*exec.ExitError); !ok {
					t.Fatalf("Incorrect error: %s\n", err)
				}
			})

			when("modifying the env fails", func() {
				var appendErr error

				it.Before(func() {
					appendErr = errors.New("some error")
				})

				each(it, []func(){
					func() {
						env.EXPECT().AddRootDir(gomock.Any()).Return(appendErr)
					},
					func() {
						env.EXPECT().AddRootDir(gomock.Any()).Return(nil)
						env.EXPECT().AddRootDir(gomock.Any()).Return(appendErr)
					},
					func() {
						env.EXPECT().AddRootDir(gomock.Any()).Return(nil)
						env.EXPECT().AddRootDir(gomock.Any()).Return(nil)
						env.EXPECT().AddEnvDir(gomock.Any()).Return(appendErr)
					},
					func() {
						env.EXPECT().AddRootDir(gomock.Any()).Return(nil)
						env.EXPECT().AddRootDir(gomock.Any()).Return(nil)
						env.EXPECT().AddEnvDir(gomock.Any()).Return(nil)
						env.EXPECT().AddEnvDir(gomock.Any()).Return(appendErr)
					},
				}, "should error", func() {
					env.EXPECT().List().Return([]string{"ID=1"})
					mkdir(t,
						filepath.Join(appDir, "buildpack1", "layer1"),
						filepath.Join(appDir, "buildpack1", "layer2"),
					)
					mkfile(t, "build = true",
						filepath.Join(appDir, "buildpack1", "layer1.toml"),
						filepath.Join(appDir, "buildpack1", "layer2.toml"),
					)
					if _, err := builder.Build(); err != appendErr {
						t.Fatalf("Incorrect error: %s\n", err)
					}
				})
			})

			it("should error when launch.toml is not writable", func() {
				env.EXPECT().List().Return([]string{"ID=1"})
				mkdir(t, filepath.Join(layersDir, "buildpack1-id", "launch.toml"))
				if _, err := builder.Build(); err == nil {
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

func rdfile(t *testing.T, path string) string {
	t.Helper()
	out, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("Error: %s\n", err)
	}
	return string(out)
}

func testExists(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func testPlan(t *testing.T, plan lifecycle.Plan, paths ...string) {
	t.Helper()
	for _, p := range paths {
		var c lifecycle.Plan
		if _, err := toml.DecodeFile(p, &c); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
		if s := cmp.Diff(c, plan); s != "" {
			t.Fatalf("Unexpected plan:\n%s\n", s)
		}
	}
}

func each(it spec.S, befores []func(), text string, f func()) {
	for i, before := range befores {
		it(fmt.Sprintf("%s #%d", text, i), func() { before(); f() })
	}
}
