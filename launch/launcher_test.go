package launch_test

import (
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/launch"
	"github.com/buildpacks/lifecycle/launch/testmock"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

//go:generate mockgen -package testmock -destination testmock/launch_env.go github.com/buildpacks/lifecycle/launch Env
//go:generate mockgen -package testmock -destination testmock/launch_execd.go github.com/buildpacks/lifecycle/launch ExecD

func TestLauncher(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
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
		env                 *testmock.MockEnv
		execd               *testmock.MockExecD
		tmpDir              string
		syscallExecArgsColl []syscallExecArgs
		envList             = []string{"TEST_ENV_ONE=1", "TEST_ENV_TWO=2"}
		wd                  string
	)

	it.Before(func() {
		mockCtrl = gomock.NewController(t)
		env = testmock.NewMockEnv(mockCtrl)
		execd = testmock.NewMockExecD(mockCtrl)
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
			Buildpacks: []launch.Buildpack{
				{API: "0.3", ID: "0.3/buildpack"},
				{API: "0.4", ID: "0.4/buildpack"},
				{API: "0.5", ID: "0.5/buildpack"},
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
				Command: "command",
				Args:    []string{"arg1", "arg2"},
			}
		})

		when("Direct=true", func() {
			var setPath string

			it.Before(func() {
				process.Direct = true

				// set command to something on the real path so exec.LookPath succeeds
				if runtime.GOOS == "windows" {
					process.Command = "notepad"
				} else {
					process.Command = "sh"
				}

				env.EXPECT().Get("PATH").Return("some-path").AnyTimes()
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
					h.AssertEq(t, syscallExecArgsColl[0].argv0, "/bin/sh")
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

			when("buildpacks have provided layer directories that could affect the environment", func() {
				it.Before(func() {
					mkdir(t,
						filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1"),
						filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2"),
						filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3"),
						filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4"),
						filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5"),
					)
				})

				it("should apply env modifications", func() {
					gomock.InOrder(
						env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1")),
						env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env.launch")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env.launch")),

						env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3")),
						env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env.launch")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env.launch")),

						env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "env")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "env.launch")),
					)
					h.AssertNil(t, launcher.LaunchProcess("", process))
					if len(syscallExecArgsColl) != 1 {
						t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
					}
				})

				when("process is buildpack-provided", func() {
					it.Before(func() {
						process.Type = "start"
					})

					it("should apply process-specific env modifications", func() {
						gomock.InOrder(
							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1")),
							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env.launch")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env.launch", "start")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env.launch")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env.launch", "start")),

							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3")),
							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env.launch")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env.launch", "start")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env.launch")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env.launch", "start")),

							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "env.launch")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "env.launch", "start")),
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
							filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "exec.d"),
							filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "exec.d", "start"),
							filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "exec.d"),
							filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "exec.d", "start"),
						)
						mkfile(t, "",
							filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "exec.d", "exec_d_1"),
							filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "exec.d", "start", "exec_d_1"),
							filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "exec.d", "exec_d_1"),
							filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "exec.d", "exec_d_2"),
							filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "exec.d", "start", "exec_d_1"),
							filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "exec.d", "start", "exec_d_2"),
						)
					})

					it("should run exec.d binaries after static env files", func() {
						gomock.InOrder(
							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1")),
							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env.launch")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env.launch")),

							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3")),
							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env.launch")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env.launch")),

							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "env.launch")),

							execd.EXPECT().ExecD(
								filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "exec.d", "exec_d_1"),
								env,
							),
							execd.EXPECT().ExecD(
								filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "exec.d", "exec_d_2"),
								env,
							),
						)
						h.AssertNil(t, launcher.LaunchProcess("", process))
						if len(syscallExecArgsColl) != 1 {
							t.Fatalf("expected syscall.Exec to be called once: actual %v\n", syscallExecArgsColl)
						}
					})

					when("process is buildpack-provided", func() {
						it.Before(func() {
							process.Type = "start"
						})

						it("should run process-specific exec.d binaries", func() {
							gomock.InOrder(
								env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1")),
								env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env.launch")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env.launch", "start")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env.launch")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env.launch", "start")),

								env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3")),
								env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env.launch")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env.launch", "start")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env.launch")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env.launch", "start")),

								env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "env")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "env.launch")),
								env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "env.launch", "start")),

								execd.EXPECT().ExecD(
									filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "exec.d", "exec_d_1"),
									env,
								),
								execd.EXPECT().ExecD(
									filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "exec.d", "exec_d_2"),
									env,
								),
								execd.EXPECT().ExecD(
									filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "exec.d", "start", "exec_d_1"),
									env,
								),
								execd.EXPECT().ExecD(
									filepath.Join(tmpDir, "launch", "0.5_buildpack", "layer5", "exec.d", "start", "exec_d_2"),
									env,
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

			when("buildpack have provided profile scripts", func() {
				it.Before(func() {
					mkdir(t,
						filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer", "profile.d"),
						filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer", "profile.d", "start"),
						filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer", "profile.d"),
						filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer", "profile.d", "start"),
					)

					if runtime.GOOS == "windows" {
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer", "profile.d", "prof1.bat"))
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer", "profile.d", "start", "prof1.bat"))
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer", "profile.d", "prof2.bat"))
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer", "profile.d", "start", "prof2.bat"))
					} else {
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer", "profile.d", "prof1"))
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer", "profile.d", "start", "prof1"))
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer", "profile.d", "prof2"))
						mkfile(t, "", filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer", "profile.d", "start", "prof2"))
					}

					env.EXPECT().AddRootDir(gomock.Any()).AnyTimes()
					env.EXPECT().AddEnvDir(gomock.Any()).AnyTimes()
				})

				it("sets the profiles", func() {
					h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
					h.AssertEq(t, shell.nCalls, 1)
					if runtime.GOOS == "windows" {
						h.AssertEq(t, shell.process.Profiles, []string{
							filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer", "profile.d", "prof1.bat"),
							filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer", "profile.d", "prof2.bat"),
						})
					} else {
						h.AssertEq(t, shell.process.Profiles, []string{
							filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer", "profile.d", "prof1"),
							filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer", "profile.d", "prof2"),
						})
					}
				})

				when("process has a type", func() {
					it.Before(func() {
						process.Type = "start"
					})

					it("includes type-specifc  profiles", func() {
						h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
						h.AssertEq(t, shell.nCalls, 1)
						if runtime.GOOS == "windows" {
							h.AssertEq(t, shell.process.Profiles, []string{
								filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer", "profile.d", "prof1.bat"),
								filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer", "profile.d", "start", "prof1.bat"),
								filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer", "profile.d", "prof2.bat"),
								filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer", "profile.d", "start", "prof2.bat"),
							})
						} else {
							h.AssertEq(t, shell.process.Profiles, []string{
								filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer", "profile.d", "prof1"),
								filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer", "profile.d", "start", "prof1"),
								filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer", "profile.d", "prof2"),
								filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer", "profile.d", "start", "prof2"),
							})
						}
					})
				})
			})

			when("buildpacks have provided layer directories that could affect the environment", func() {
				it.Before(func() {
					mkdir(t,
						filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1"),
						filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2"),
						filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3"),
						filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4"),
					)
				})

				it("should apply env modifications", func() {
					gomock.InOrder(
						env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1")),
						env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env.launch")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env.launch")),

						env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3")),
						env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env.launch")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env")),
						env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env.launch")),
					)
					h.AssertNil(t, launcher.LaunchProcess("/path/to/launcher", process))
					h.AssertEq(t, shell.nCalls, 1)
				})

				when("process has a type", func() {
					it.Before(func() {
						process.Type = "start"
					})

					it("should apply type-specific env modifications", func() {
						gomock.InOrder(
							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1")),
							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env.launch")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer1", "env.launch", "start")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env.launch")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.3_buildpack", "layer2", "env.launch", "start")),

							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3")),
							env.EXPECT().AddRootDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env.launch")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer3", "env.launch", "start")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env.launch")),
							env.EXPECT().AddEnvDir(filepath.Join(tmpDir, "launch", "0.4_buildpack", "layer4", "env.launch", "start")),
						)
						h.AssertNil(t, launcher.LaunchProcess("", process))
						h.AssertEq(t, shell.nCalls, 1)
					})
				})
			})

			when("buildpack-provided", func() {
				when("buildpack API >= 0.4", func() {
					it.Before(func() {
						process.BuildpackID = "0.4/buildpack"
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

				when("buildpack API < 0.4", func() {
					it.Before(func() {
						process.BuildpackID = "0.3/buildpack"
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
