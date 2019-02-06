package lifecycle_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/golang/mock/gomock"

	"github.com/buildpack/lifecycle"
	"github.com/buildpack/lifecycle/testmock"
)

func TestLauncher(t *testing.T) {
	spec.Run(t, "Launcher", testLauncher, spec.Report(report.Terminal{}))
}

type syscallExecArgs struct {
	argv0 string
	argv  []string
	envv  []string
}

func testLauncher(t *testing.T, when spec.G, it spec.S) {
	var (
		launcher            *lifecycle.Launcher
		mockCtrl            *gomock.Controller
		env                 *testmock.MockBuildEnv
		tmpDir              string
		syscallExecArgsColl []syscallExecArgs
		wd                  string
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		env = testmock.NewMockBuildEnv(mockCtrl)
		env.EXPECT().List().Return([]string{"TEST_ENV_ONE=1", "TEST_ENV_TWO=2"}).AnyTimes()

		var err error
		tmpDir, err = ioutil.TempDir("", "lifecycle.launcher.")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(tmpDir, "launch", "app"), 0755); err != nil {
			t.Fatal(err)
		}
		launcher = &lifecycle.Launcher{
			DefaultProcessType: "web",
			LayersDir:          filepath.Join(tmpDir, "launch"),
			AppDir:             filepath.Join(tmpDir, "launch", "app"),
			Processes: []lifecycle.Process{
				{Type: "other", Command: "some-other-process"},
				{Type: "web", Command: "some-web-process"},
				{Type: "worker", Command: "some-worker-process"},
			},
			Buildpacks: []string{},
			Env:        env,
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
				if err := launcher.Launch("/path/to/launcher", ""); err != nil {
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
			})

			when("default start process type is not in the process types", func() {
				it("should return an error", func() {
					launcher.DefaultProcessType = "not-exist"

					if err := launcher.Launch("/path/to/launcher", ""); err == nil {
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
					if err := launcher.Launch("/path/to/launcher", "worker"); err != nil {
						t.Fatal(err)
					}

					if len(syscallExecArgsColl) != 1 {
						t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
					}

					if diff := cmp.Diff(syscallExecArgsColl[0].argv[4], "some-worker-process"); diff != "" {
						t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
					}
				})
			})

			when("start command does NOT match a process type", func() {
				it("should run the start command", func() {
					if err := launcher.Launch("/path/to/launcher", "some-different-process"); err != nil {
						t.Fatal(err)
					}

					if len(syscallExecArgsColl) != 1 {
						t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
					}

					if diff := cmp.Diff(syscallExecArgsColl[0].argv[4], "some-different-process"); diff != "" {
						t.Fatalf("syscall.Exec Argv did not match: (-got +want)\n%s\n", diff)
					}
				})
			})
		})

		when("buildpacks have provided layer directories that could affect the environment", func() {
			it.Before(func() {
				mkfile(t, "#!/usr/bin/env bash\necho test1: $TEST_ENV_ONE test2: $TEST_ENV_TWO\n",
					filepath.Join(tmpDir, "launch", "app", "start"),
				)

				launcher.Processes = []lifecycle.Process{
					{Type: "start", Command: "./start"},
				}
				launcher.Buildpacks = []string{"bp.1", "bp.2"}
				launcher.Exec = syscallExecWithStdout(t, tmpDir)

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
				if err := launcher.Launch("/path/to/launcher", "start"); err != nil {
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
				launcher.Buildpacks = []string{"bp.3"}
			})

			it("ignores those buildpacks when setting the env", func() {
				if err := launcher.Launch("/path/to/launcher", "start"); err != nil {
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

				launcher.Processes = []lifecycle.Process{
					{Type: "start", Command: "./start"},
				}
				launcher.Buildpacks = []string{"bp.1", "bp.2"}
				launcher.Exec = syscallExecWithStdout(t, tmpDir)

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
				if err := launcher.Launch("/path/to/launcher", "start"); err != nil {
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
					launcher.Buildpacks = []string{"bp.2", "bp.1"}
				})

				it("should run them in buildpack order", func() {
					if err := launcher.Launch("/path/to/launcher", "start"); err != nil {
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
					if err := launcher.Launch("/path/to/launcher", "start"); err != nil {
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

func syscallExecWithStdout(t *testing.T, tmpDir string) func(argv0 string, argv []string, envv []string) error {
	t.Helper()
	fstdin, err := os.Create(filepath.Join(tmpDir, "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	fstdout, err := os.Create(filepath.Join(tmpDir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	fstderr, err := os.Create(filepath.Join(tmpDir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}

	return func(argv0 string, argv []string, envv []string) error {
		pid, err := syscall.ForkExec(argv0, argv, &syscall.ProcAttr{
			Dir:   filepath.Join(tmpDir, "launch", "app"),
			Env:   envv,
			Files: []uintptr{fstdin.Fd(), fstdout.Fd(), fstderr.Fd()},
			Sys: &syscall.SysProcAttr{
				Foreground: false,
			},
		})
		if err != nil {
			return err
		}
		_, err = syscall.Wait4(pid, nil, 0, nil)
		return err
	}
}
