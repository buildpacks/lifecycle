package launch_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/launch/testmock"
	"github.com/buildpacks/lifecycle/launch/testsyscall"
)

//go:generate mockgen -package testmock -destination testmock/launch_env.go github.com/buildpacks/lifecycle/launch Env

func TestLauncher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping launcher tests for Windows")
	}

	spec.Run(t, "Launcher", testLauncher, spec.Report(report.Terminal{}))
}

type syscallExecArgs struct {
	argv0 string
	argv  []string
	envv  []string
}

func testLauncher(t *testing.T, when spec.G, it spec.S) {
	var (
		launcher            *launch.Launcher
		mockCtrl            *gomock.Controller
		env                 *testmock.MockEnv
		tmpDir              string
		syscallExecArgsColl []syscallExecArgs
		wd                  string
		envList             = []string{"TEST_ENV_ONE=1", "TEST_ENV_TWO=2"}
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		env = testmock.NewMockEnv(mockCtrl)
		env.EXPECT().List().Return(envList).AnyTimes()

		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle.launcher.")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(tmpDir, "launch", "app"), 0755); err != nil {
			t.Fatal(err)
		}
		launcher = &launch.Launcher{
			DefaultProcessType: "web",
			LayersDir:          filepath.Join(tmpDir, "launch"),
			AppDir:             filepath.Join(tmpDir, "launch", "app"),
			Processes: []launch.Process{
				{Type: "other", Command: "some-other-process"},
				{Type: "web", Command: "some-web-process", Args: []string{"arg1", "arg2"}},
				{Type: "worker", Command: "some-worker-process"},
				{Type: "direct", Command: "sh", Args: []string{"arg1", "arg2"}, Direct: true},
			},
			Env: env,
			Exec: func(argv0 string, argv []string, envv []string) error {
				syscallExecArgsColl = append(syscallExecArgsColl, syscallExecArgs{
					argv0: argv0,
					argv:  argv,
					envv:  envv,
				})
				return nil
			},
		}
		wd, err = os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
	})

	it.After(func() {
		os.Chdir(wd) // restore the working dir after Launcher changes it
		os.RemoveAll(tmpDir)
		mockCtrl.Finish()
	})

	when("#Launch", func() {
		when("no start command has been specified", func() {
			it("should run the default process type", func() {
				if err := launcher.Launch("/path/to/launcher", nil); err != nil {
					t.Fatal(err)
				}

				if len(syscallExecArgsColl) != 1 {
					t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
				}

				if diff := cmp.Diff(syscallExecArgsColl[0].argv0, "/bin/bash"); diff != "" {
					t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
				}

				if diff := cmp.Diff(syscallExecArgsColl[0].argv[3], "/path/to/launcher"); diff != "" {
					t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
				}

				if diff := cmp.Diff(syscallExecArgsColl[0].argv[4], "some-web-process"); diff != "" {
					t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
				}
				if diff := cmp.Diff(syscallExecArgsColl[0].argv[5], "arg1"); diff != "" {
					t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
				}
				if diff := cmp.Diff(syscallExecArgsColl[0].argv[6], "arg2"); diff != "" {
					t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
				}
				if diff := cmp.Diff(syscallExecArgsColl[0].envv, envList); diff != "" {
					t.Fatalf("syscall.Exec envv did not match: (-got +want)\n%s\n", diff)
				}
			})

			when("default start process type is not in the process types", func() {
				it("should return an error", func() {
					launcher.DefaultProcessType = "not-exist"

					if err := launcher.Launch("/path/to/launcher", nil); err == nil {
						t.Fatal("expected launch to return an error")
					}

					if len(syscallExecArgsColl) != 0 {
						t.Fatalf("expected syscall.Exec to not be called: actual %v\n", syscallExecArgsColl)
					}
				})
			})
		})

		when("start command has been specified", func() {
			when("start command matches a process type", func() {
				it("should run that process type", func() {
					if err := launcher.Launch("/path/to/launcher", []string{"worker"}); err != nil {
						t.Fatal(err)
					}

					if len(syscallExecArgsColl) != 1 {
						t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
					}

					if diff := cmp.Diff(syscallExecArgsColl[0].argv[4], "some-worker-process"); diff != "" {
						t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
					}
					if diff := cmp.Diff(syscallExecArgsColl[0].envv, envList); diff != "" {
						t.Fatalf("syscall.Exec envv did not match: (-got +want)\n%s\n", diff)
					}
				})
			})

			when("start command does NOT match a process type", func() {
				it("should run the start command", func() {
					if err := launcher.Launch("/path/to/launcher", []string{"some-different-process"}); err != nil {
						t.Fatal(err)
					}

					if len(syscallExecArgsColl) != 1 {
						t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
					}

					if diff := cmp.Diff(syscallExecArgsColl[0].argv[4], "some-different-process"); diff != "" {
						t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
					}
					if diff := cmp.Diff(syscallExecArgsColl[0].envv, envList); diff != "" {
						t.Fatalf("syscall.Exec envv did not match: (-got +want)\n%s\n", diff)
					}
				})
			})
		})

		when("a start command is marked as direct", func() {
			var setPath string

			it.Before(func() {
				env.EXPECT().Get("PATH").Return("some-path")
				launcher.Setenv = func(k string, v string) error {
					if k == "PATH" {
						setPath = v
					}
					return nil
				}
			})

			it("should invoke a process type's start command directly", func() {
				if err := launcher.Launch("/path/to/launcher", []string{"direct"}); err != nil {
					t.Fatal(err)
				}

				if len(syscallExecArgsColl) != 1 {
					t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
				}

				if diff := cmp.Diff(setPath, "some-path"); diff != "" {
					t.Fatalf("launcher did not set PATH: (-got +want)\n%s\n", diff)
				}
				if diff := cmp.Diff(syscallExecArgsColl[0].argv0, "/bin/sh"); diff != "" {
					t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
				}

				if diff := cmp.Diff(syscallExecArgsColl[0].argv[0], "sh"); diff != "" {
					t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
				}
				if diff := cmp.Diff(syscallExecArgsColl[0].argv[1], "arg1"); diff != "" {
					t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
				}
				if diff := cmp.Diff(syscallExecArgsColl[0].argv[2], "arg2"); diff != "" {
					t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
				}
				if diff := cmp.Diff(syscallExecArgsColl[0].envv, envList); diff != "" {
					t.Fatalf("syscall.Exec envv did not match: (-got +want)\n%s\n", diff)
				}
			})

			it("should invoke a provided start command directly", func() {
				if err := launcher.Launch("/path/to/launcher", []string{"--", "sh", "arg1", "arg2"}); err != nil {
					t.Fatal(err)
				}

				if diff := cmp.Diff(setPath, "some-path"); diff != "" {
					t.Fatalf("launcher did not set PATH: (-got +want)\n%s\n", diff)
				}
				if len(syscallExecArgsColl) != 1 {
					t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
				}

				if diff := cmp.Diff(syscallExecArgsColl[0].argv0, "/bin/sh"); diff != "" {
					t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
				}

				if diff := cmp.Diff(syscallExecArgsColl[0].argv[0], "sh"); diff != "" {
					t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
				}
				if diff := cmp.Diff(syscallExecArgsColl[0].argv[1], "arg1"); diff != "" {
					t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
				}
				if diff := cmp.Diff(syscallExecArgsColl[0].argv[2], "arg2"); diff != "" {
					t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
				}
				if diff := cmp.Diff(syscallExecArgsColl[0].envv, envList); diff != "" {
					t.Fatalf("syscall.Exec envv did not match: (-got +want)\n%s\n", diff)
				}
			})
		})

		when("buildpacks have provided layer directories that could affect the environment", func() {
			it.Before(func() {
				mkfile(t, "#!/usr/bin/env bash\necho test1: $TEST_ENV_ONE test2: $TEST_ENV_TWO\n",
					filepath.Join(tmpDir, "launch", "app", "start"),
				)

				launcher.Processes = []launch.Process{
					{Type: "start", Command: "./start"},
				}
				launcher.Buildpacks = []launch.Buildpack{{ID: "bp.1"}, {ID: "bp.2"}}
				launcher.Exec = testsyscall.SyscallExecWithStdout(t, tmpDir)

				mkdir(t,
					filepath.Join(tmpDir, "launch", "bp.1", "layer1"),
					filepath.Join(tmpDir, "launch", "bp.1", "layer2"),
					filepath.Join(tmpDir, "launch", "bp.2", "layer3"),
					filepath.Join(tmpDir, "launch", "bp.2", "layer4"),
				)
			})

			it("should ensure each buildpack's layers dir exists and process build layers", func() {
				gomock.InOrder(
					env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "bp.1", "layer1")),
					env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "bp.1", "layer2")),
					env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "bp.1", "layer1", "env")),
					env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "bp.1", "layer1", "env.launch")),
					env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "bp.1", "layer2", "env")),
					env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "bp.1", "layer2", "env.launch")),

					env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "bp.2", "layer3")),
					env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "bp.2", "layer4")),
					env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "bp.2", "layer3", "env")),
					env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "bp.2", "layer3", "env.launch")),
					env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "bp.2", "layer4", "env")),
					env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "bp.2", "layer4", "env.launch")),
				)
				if err := launcher.Launch("/path/to/launcher", []string{"start"}); err != nil {
					t.Fatal(err)
				}
				stdout := rdfile(t, filepath.Join(tmpDir, "stdout"))
				if len(stdout) == 0 {
					stderr := rdfile(t, filepath.Join(tmpDir, "stderr"))
					t.Fatalf("stdout was empty: stderr: %s\n", stderr)
				}
				if diff := cmp.Diff(stdout, "test1: 1 test2: 2\n"); diff != "" {
					t.Fatalf("syscall.Exec stdout did not match: (-got +want)\n%s\n", diff)
				}
			})
		})

		when("metadata includes buildpacks that have not contributed layers", func() {
			it.Before(func() {
				launcher.Buildpacks = []launch.Buildpack{{ID: "bp.3"}}
			})

			it("ignores those buildpacks when setting the env", func() {
				if err := launcher.Launch("/path/to/launcher", []string{"start"}); err != nil {
					t.Fatal(err)
				}
				if len(syscallExecArgsColl) != 1 {
					t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
				}
			})
		})

		when("buildpacks have provided profile.d scripts", func() {
			it.Before(func() {
				mkfile(t, "#!/usr/bin/env bash\necho hi from app\n",
					filepath.Join(tmpDir, "launch", "app", "start"),
				)

				launcher.Processes = []launch.Process{
					{Type: "start", Command: "./start"},
				}
				launcher.Buildpacks = []launch.Buildpack{{ID: "bp.1"}, {ID: "bp.2"}}
				launcher.Exec = testsyscall.SyscallExecWithStdout(t, tmpDir)

				mkdir(t,
					filepath.Join(tmpDir, "launch", "bp.1", "layer", "profile.d"),
					filepath.Join(tmpDir, "launch", "bp.2", "layer", "profile.d"),
				)
				mkfile(t, "echo apple", filepath.Join(tmpDir, "launch", "bp.1", "layer", "profile.d", "apple"))
				mkfile(t, "echo banana", filepath.Join(tmpDir, "launch", "bp.2", "layer", "profile.d", "banana"))

				env.EXPECT().AddRootDir(gomock.Any()).AnyTimes()
				env.EXPECT().AddEnvDir(gomock.Any()).AnyTimes()
			})

			it("should run them in buildpack order", func() {
				if err := launcher.Launch("/path/to/launcher", []string{"start"}); err != nil {
					t.Fatal(err)
				}

				stdout := rdfile(t, filepath.Join(tmpDir, "stdout"))
				if len(stdout) == 0 {
					stderr := rdfile(t, filepath.Join(tmpDir, "stderr"))
					t.Fatalf("stdout was empty: stderr: %s\n", stderr)
				}
				if diff := cmp.Diff(stdout, "apple\nbanana\nhi from app\n"); diff != "" {
					t.Fatalf("syscall.Exec stdout did not match: (-got +want)\n%s\n", diff)
				}
			})

			when("changing the buildpack order", func() {
				it.Before(func() {
					launcher.Buildpacks = []launch.Buildpack{{ID: "bp.2"}, {ID: "bp.1"}}
				})

				it("should run them in buildpack order", func() {
					if err := launcher.Launch("/path/to/launcher", []string{"start"}); err != nil {
						t.Fatal(err)
					}

					stdout := rdfile(t, filepath.Join(tmpDir, "stdout"))
					if len(stdout) == 0 {
						stderr := rdfile(t, filepath.Join(tmpDir, "stderr"))
						t.Fatalf("stdout was empty: stderr: %s\n", stderr)
					}
					if diff := cmp.Diff(stdout, "banana\napple\nhi from app\n"); diff != "" {
						t.Fatalf("syscall.Exec stdout did not match: (-got +want)\n%s\n", diff)
					}
				})
			})

			when("app has '.profile'", func() {
				it.Before(func() {
					mkfile(t, "echo from profile",
						filepath.Join(tmpDir, "launch", "app", ".profile"),
					)
				})

				it("should source .profile", func() {
					if err := launcher.Launch("/path/to/launcher", []string{"start"}); err != nil {
						t.Fatal(err)
					}

					stdout := rdfile(t, filepath.Join(tmpDir, "stdout"))
					if len(stdout) == 0 {
						stderr := rdfile(t, filepath.Join(tmpDir, "stderr"))
						t.Fatalf("stdout was empty: stderr: %s\n", stderr)
					}
					if diff := cmp.Diff(stdout, "apple\nbanana\nfrom profile\nhi from app\n"); diff != "" {
						t.Fatalf("syscall.Exec stdout did not match: (-got +want)\n%s\n", diff)
					}
				})
			})
		})
	})
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

func mkdir(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0777); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}
