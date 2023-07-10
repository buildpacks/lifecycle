package launch_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/env"
	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/launch/testmock"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

//go:generate mockgen -package testmock -destination testmock/launch_env.go github.com/buildpacks/lifecycle/launch Env
//go:generate mockgen -package testmock -destination testmock/launch_execd.go github.com/buildpacks/lifecycle/launch ExecD

func TestLauncher(t *testing.T) {
	spec.Run(t, "Launcher", testLauncher, spec.Sequential(), spec.Report(report.Terminal{}))
}

type syscallExecArgs struct {
	argv0 string
	argv  []string
	envv  []string
}

type fakeShell struct {
	nCalls  int
	process launch.ShellProcess
}

func (fs *fakeShell) Launch(sp launch.ShellProcess) error {
	fs.nCalls++
	fs.process = sp
	return nil
}

func testLauncher(t *testing.T, when spec.G, it spec.S) {
	var (
		launcher            *launch.Launcher
		mockCtrl            *gomock.Controller
		mockEnv             *testmock.MockEnv
		execd               *testmock.MockExecD
		tmpDir              string
		syscallExecArgsColl []syscallExecArgs
		envList             = []string{"TEST_ENV_ONE=1", "TEST_ENV_TWO=2"}
		wd                  string
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		mockEnv = testmock.NewMockEnv(mockCtrl)
		execd = testmock.NewMockExecD(mockCtrl)
		mockEnv.EXPECT().List().Return(envList).AnyTimes()

		var err error
		tmpDir, err = os.MkdirTemp("", "lifecycle.launcher.")
		if err != nil {
			t.Fatal(err)
		}
		// MacOS can have temp dir in a symlink, which breaks path comparisons.
		tmpDir, err = filepath.EvalSymlinks(tmpDir)
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
			Buildpacks: []launch.Buildpack{
				{API: "0.7", ID: "no-layers/buildpack"}, // TODO
				{API: "0.7", ID: "0.7/buildpack"},
				{API: "0.8", ID: "0.8/buildpack"},
				{API: "0.9", ID: "0.9/buildpack"},
			},
			Env: mockEnv,
			Exec: func(argv0 string, argv []string, envv []string) error {
				syscallExecArgsColl = append(syscallExecArgsColl, syscallExecArgs{
					argv0: argv0,
					argv:  argv,
					envv:  envv,
				})
				return nil
			},
			ExecD: execd,
		}
		wd, err = os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
	})

	it.After(func() {
		h.AssertNil(t, os.Chdir(wd)) // restore the working dir after Launcher changes it
		mockCtrl.Finish()
		h.AssertNil(t, os.RemoveAll(tmpDir))
	})

	when("LaunchProcess", func() {
		var process launch.Process

		it.Before(func() {
			process = launch.Process{
				Command: launch.NewRawCommand([]string{"command"}),
				Args:    []string{"arg1", "arg2"},
			}
		})

		when("Direct=true", func() {
			var setPath string

			it.Before(func() {
				process.Direct = true

				// set command to something on the real path so exec.LookPath succeeds
				if runtime.GOOS == "windows" {
					process.Command = launch.NewRawCommand([]string{"notepad"})
				} else {
					process.Command = launch.NewRawCommand([]string{"sh"})
				}

				mockEnv.EXPECT().Get("PATH").Return("some-path").AnyTimes()
				launcher.Setenv = func(k string, v string) error {
					if k == "PATH" {
						setPath = v
					}
					return nil
				}
			})

			it("should set the path", func() {
				h.AssertNil(t, launcher.LaunchProcess("", process))
				if len(syscallExecArgsColl) != 1 {
					t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
				}
				if diff := cmp.Diff(setPath, "some-path"); diff != "" {
					t.Fatalf("launcher did not set PATH: (-got +want)\n%s\n", diff)
				}
			})

			it("should set argv0 to absolute path of command", func() {
				h.AssertNil(t, launcher.LaunchProcess("", process))
				if len(syscallExecArgsColl) != 1 {
					t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
				}
				if runtime.GOOS == "windows" {
					h.AssertEq(t, strings.ToLower(syscallExecArgsColl[0].argv0), `c:\windows\system32\notepad.exe`)
				} else {
					h.AssertMatch(t, syscallExecArgsColl[0].argv0, ".*/bin/sh$")
				}
			})

			it("should set argv to command+args", func() {
				h.AssertNil(t, launcher.LaunchProcess("", process))
				if len(syscallExecArgsColl) != 1 {
					t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
				}
				if runtime.GOOS == "windows" {
					h.AssertEq(t, syscallExecArgsColl[0].argv, []string{"notepad", "arg1", "arg2"})
				} else {
					h.AssertEq(t, syscallExecArgsColl[0].argv, []string{"sh", "arg1", "arg2"})
				}
			})

			it("should set envv", func() {
				h.AssertNil(t, launcher.LaunchProcess("", process))
				if len(syscallExecArgsColl) != 1 {
					t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
				}
				h.AssertEq(t, syscallExecArgsColl[0].envv, envList)
			})

			it("should default the working directory to the app directory", func() {
				process.WorkingDirectory = ""
				h.AssertNil(t, launcher.LaunchProcess("", process))
				actualDir, err := os.Getwd()
				h.AssertNil(t, err)
				h.AssertEq(t, actualDir, launcher.AppDir)
			})

			it("should execute in the specified working directory", func() {
				process.WorkingDirectory = tmpDir
				h.AssertNil(t, launcher.LaunchProcess("", process))
				actualDir, err := os.Getwd()
				h.AssertNil(t, err)
				h.AssertEq(t, actualDir, tmpDir)
			})

			when("buildpacks have provided layer directories that could affect the environment", func() {
				it.Before(func() {
					mkdir(t,
						filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1"),
						filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2"),
						filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3"),
						filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4"),
						filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5"),
					)
				})

				it("should apply env modifications", func() {
					gomock.InOrder(
						mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1")),
						mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2")),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env.launch"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env.launch"), env.ActionTypeOverride),

						mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3")),
						mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4")),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env.launch"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env.launch"), env.ActionTypeOverride),

						mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5")),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env.launch"), env.ActionTypeOverride),
					)
					h.AssertNil(t, launcher.LaunchProcess("", process))
					if len(syscallExecArgsColl) != 1 {
						t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
					}
				})

				when("process is buildpack-provided", func() {
					it.Before(func() {
						process.Type = "some-process-type"
					})

					it("should apply process-specific env modifications", func() {
						gomock.InOrder(
							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1")),
							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2")),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env.launch"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env.launch", "some-process-type"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env.launch"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env.launch", "some-process-type"), env.ActionTypeOverride),

							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3")),
							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4")),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env.launch"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env.launch", "some-process-type"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env.launch"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env.launch", "some-process-type"), env.ActionTypeOverride),

							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5")),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env.launch"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env.launch", "some-process-type"), env.ActionTypeOverride),
						)
						h.AssertNil(t, launcher.LaunchProcess("", process))
						if len(syscallExecArgsColl) != 1 {
							t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
						}
					})
				})

				when("buildpack API supports exec.d", func() {
					it.Before(func() {
						mkdir(t,
							filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "exec.d"),
							filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "exec.d", "some-process-type"),
							filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "exec.d"),
							filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "exec.d", "some-process-type"),
						)

						// exec.d binaries from buildpacks with API >= 0.9 should be executed
						mkfile(t, "",
							filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "exec.d", "exec_d_1"),
							filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "exec.d", "exec_d_2"),
							filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "exec.d", "some-process-type", "exec_d_1"),
							filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "exec.d", "some-process-type", "exec_d_2"),
						)
					})

					it("should run exec.d binaries after static env files", func() {
						gomock.InOrder(
							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1")),
							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2")),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env.launch"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env.launch"), env.ActionTypeOverride),

							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3")),
							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4")),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env.launch"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env.launch"), env.ActionTypeOverride),

							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5")),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env.launch"), env.ActionTypeOverride),

							execd.EXPECT().ExecD(
								filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "exec.d", "exec_d_1"),
								mockEnv,
							),
							execd.EXPECT().ExecD(
								filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "exec.d", "exec_d_2"),
								mockEnv,
							),
						)
						h.AssertNil(t, launcher.LaunchProcess("", process))
						if len(syscallExecArgsColl) != 1 {
							t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
						}
					})

					when("process is buildpack-provided", func() {
						it.Before(func() {
							process.Type = "some-process-type"
						})

						it("should run process-specific exec.d binaries", func() {
							gomock.InOrder(
								mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1")),
								mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2")),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env"), env.ActionTypeOverride),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env.launch"), env.ActionTypeOverride),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env.launch", "some-process-type"), env.ActionTypeOverride),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env"), env.ActionTypeOverride),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env.launch"), env.ActionTypeOverride),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env.launch", "some-process-type"), env.ActionTypeOverride),

								mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3")),
								mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4")),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env"), env.ActionTypeOverride),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env.launch"), env.ActionTypeOverride),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env.launch", "some-process-type"), env.ActionTypeOverride),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env"), env.ActionTypeOverride),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env.launch"), env.ActionTypeOverride),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env.launch", "some-process-type"), env.ActionTypeOverride),

								mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5")),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env"), env.ActionTypeOverride),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env.launch"), env.ActionTypeOverride),
								mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env.launch", "some-process-type"), env.ActionTypeOverride),

								execd.EXPECT().ExecD(
									filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "exec.d", "exec_d_1"),
									mockEnv,
								),
								execd.EXPECT().ExecD(
									filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "exec.d", "exec_d_2"),
									mockEnv,
								),
								execd.EXPECT().ExecD(
									filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "exec.d", "some-process-type", "exec_d_1"),
									mockEnv,
								),
								execd.EXPECT().ExecD(
									filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "exec.d", "some-process-type", "exec_d_2"),
									mockEnv,
								),
							)
							h.AssertNil(t, launcher.LaunchProcess("", process))
							if len(syscallExecArgsColl) != 1 {
								t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
							}
						})
					})
				})
			})
		})

		when("Direct=false", func() {
			var (
				shell *fakeShell
			)
			it.Before(func() {
				shell = &fakeShell{}
				launcher.Shell = shell
			})

			it("sets Caller to self", func() {
				h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
				h.AssertEq(t, shell.nCalls, 1)
				h.AssertEq(t, shell.process.Caller, "/path/to/launcher")
			})

			it("sets command and args from process", func() {
				h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
				h.AssertEq(t, shell.nCalls, 1)
				h.AssertEq(t, shell.process.Command, "command")
				h.AssertEq(t, shell.process.Args, []string{"arg1", "arg2"})
			})

			it("sets env", func() {
				h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
				h.AssertEq(t, shell.nCalls, 1)
				h.AssertEq(t, shell.process.Env, envList)
			})

			when("process specific working directory", func() {
				it.Before(func() {
					process.WorkingDirectory = "/some-dir"
				})

				it("sets the working directory on the shell process", func() {
					h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
					h.AssertEq(t, shell.process.WorkingDirectory, "/some-dir")
				})
			})

			when("no specified working directory", func() {
				it.Before(func() {
					process.WorkingDirectory = ""
				})

				it("sets the working directory to the app directory", func() {
					h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
					h.AssertEq(t, shell.process.WorkingDirectory, launcher.AppDir)
				})
			})

			when("buildpack have provided profile scripts", func() {
				it.Before(func() {
					mkdir(t,
						filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer", "profile.d"),
						filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer", "profile.d", "some-process-type"),
						filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer", "profile.d"),
						filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer", "profile.d", "some-process-type"),
					)

					if runtime.GOOS == "windows" {
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer", "profile.d", "prof1.bat"))
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer", "profile.d", "some-process-type", "prof1.bat"))
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer", "profile.d", "prof2.bat"))
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer", "profile.d", "some-process-type", "prof2.bat"))
					} else {
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer", "profile.d", "prof1"))
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer", "profile.d", "some-process-type", "prof1"))
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer", "profile.d", "prof2"))
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer", "profile.d", "some-process-type", "prof2"))
					}

					mockEnv.EXPECT().AddRootDir(gomock.Any()).AnyTimes()
					mockEnv.EXPECT().AddEnvDir(gomock.Any(), gomock.Any()).AnyTimes()
				})

				it("sets the profiles", func() {
					h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
					h.AssertEq(t, shell.nCalls, 1)
					if runtime.GOOS == "windows" {
						h.AssertEq(t, shell.process.Profiles, []string{
							filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer", "profile.d", "prof1.bat"),
							filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer", "profile.d", "prof2.bat"),
						})
					} else {
						h.AssertEq(t, shell.process.Profiles, []string{
							filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer", "profile.d", "prof1"),
							filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer", "profile.d", "prof2"),
						})
					}
				})

				when("process has a type", func() {
					it.Before(func() {
						process.Type = "some-process-type"
					})

					it("includes type-specifc  profiles", func() {
						h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
						h.AssertEq(t, shell.nCalls, 1)
						if runtime.GOOS == "windows" {
							h.AssertEq(t, shell.process.Profiles, []string{
								filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer", "profile.d", "prof1.bat"),
								filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer", "profile.d", "some-process-type", "prof1.bat"),
								filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer", "profile.d", "prof2.bat"),
								filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer", "profile.d", "some-process-type", "prof2.bat"),
							})
						} else {
							h.AssertEq(t, shell.process.Profiles, []string{
								filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer", "profile.d", "prof1"),
								filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer", "profile.d", "some-process-type", "prof1"),
								filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer", "profile.d", "prof2"),
								filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer", "profile.d", "some-process-type", "prof2"),
							})
						}
					})
				})
			})

			when("buildpacks have provided layer directories that could affect the environment", func() {
				it.Before(func() {
					mkdir(t,
						filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1"),
						filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2"),
						filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3"),
						filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4"),
						filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5"),
					)
				})

				it("should apply env modifications", func() {
					gomock.InOrder(
						mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1")),
						mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2")),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env.launch"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env.launch"), env.ActionTypeOverride),

						mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3")),
						mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4")),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env.launch"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env.launch"), env.ActionTypeOverride),

						mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5")),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env"), env.ActionTypeOverride),
						mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env.launch"), env.ActionTypeOverride),
					)
					h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
					h.AssertEq(t, shell.nCalls, 1)
				})

				when("process has a type", func() {
					it.Before(func() {
						process.Type = "some-process-type"
					})

					it("should apply type-specific env modifications", func() {
						gomock.InOrder(
							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1")),
							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2")),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env.launch"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer1", "env.launch", "some-process-type"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env.launch"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.7_buildpack", "layer2", "env.launch", "some-process-type"), env.ActionTypeOverride),

							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3")),
							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4")),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env.launch"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer3", "env.launch", "some-process-type"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env.launch"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.8_buildpack", "layer4", "env.launch", "some-process-type"), env.ActionTypeOverride),

							mockEnv.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5")),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env.launch"), env.ActionTypeOverride),
							mockEnv.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.9_buildpack", "layer5", "env.launch", "some-process-type"), env.ActionTypeOverride),
						)
						h.AssertNil(t, launcher.LaunchProcess("", process))
						h.AssertEq(t, shell.nCalls, 1)
					})
				})
			})

			when("buildpack-provided", func() {
				it.Before(func() {
					process.BuildpackID = "0.8/buildpack"
				})

				when("has args", func() {
					it("is not script", func() {
						h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
						h.AssertEq(t, shell.nCalls, 1)

						if shell.process.Script {
							t.Fatalf("did not expect script process")
						}
					})
				})

				when("has no args", func() {
					it.Before(func() {
						process.Args = []string{}
					})

					when("linux", func() {
						it.Before(func() {
							h.SkipIf(t, runtime.GOOS == "windows", "linux test")
						})

						it("is script", func() {
							h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
							h.AssertEq(t, shell.nCalls, 1)

							if !shell.process.Script {
								t.Fatalf("expected script process")
							}
						})
					})

					when("windows", func() {
						it.Before(func() {
							h.SkipIf(t, runtime.GOOS != "windows", "windows test")
						})

						it("is not script", func() {
							h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
							h.AssertEq(t, shell.nCalls, 1)

							if shell.process.Script {
								t.Fatalf("did not expect script process")
							}
						})
					})
				})
			})

			when("user-provided", func() {
				when("has args", func() {
					it("is not script", func() {
						h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
						h.AssertEq(t, shell.nCalls, 1)

						if shell.process.Script {
							t.Fatalf("did not expect script process")
						}
					})
				})

				when("has no args", func() {
					it.Before(func() {
						process.Args = []string{}
					})

					when("linux", func() {
						it.Before(func() {
							h.SkipIf(t, runtime.GOOS == "windows", "linux test")
						})

						it("is script", func() {
							h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
							h.AssertEq(t, shell.nCalls, 1)

							if !shell.process.Script {
								t.Fatalf("expected script process")
							}
						})
					})

					when("windows", func() {
						it.Before(func() {
							h.SkipIf(t, runtime.GOOS != "windows", "windows test")
						})

						it("is not script", func() {
							h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
							h.AssertEq(t, shell.nCalls, 1)

							if shell.process.Script {
								t.Fatalf("did not expect script process")
							}
						})
					})
				})
			})
		})
	})
}

func mkfile(t *testing.T, data string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatalf("Error: %s\n", err)
		}
	}
}

func rdfile(t *testing.T, path string) string {
	t.Helper()
	out, err := os.ReadFile(path)
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
