package lifecycle_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/buildpack/lifecycle"
	"github.com/google/go-cmp/cmp"
	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"
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
		tmpDir              string
		syscallExecArgsColl []syscallExecArgs
	)

	it.Before(func() {
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
			DefaultLaunchDir:   filepath.Join(tmpDir, "launch"),
			Processes: []lifecycle.Process{
				{Type: "other", Command: "some-other-process"},
				{Type: "web", Command: "some-web-process"},
				{Type: "worker", Command: "some-worker-process"},
			},
			Buildpacks: []string{},
			Exec: func(argv0 string, argv []string, envv []string) error {
				syscallExecArgsColl = append(syscallExecArgsColl, syscallExecArgs{
					argv0: argv0,
					argv:  argv,
					envv:  envv,
				})
				return nil
			},
		}
	})

	it.After(func() {
		os.RemoveAll(tmpDir)
	})

	when("#Launch", func() {
		when("no start command has been specified", func() {
			it("should run the default process type", func() {
				if err := launcher.Launch("/path/to/launcher", ""); err != nil {
					t.Fatal(err)
				}

				if len(syscallExecArgsColl) != 1 {
					t.Fatalf("expected syscall.Exec to be called once: actual %v", syscallExecArgsColl)
				}

				if diff := cmp.Diff(syscallExecArgsColl[0].argv0, "/bin/bash"); diff != "" {
					t.Fatalf(`syscall.Exec Argv did not match: (-got +want)\n%s`, diff)
				}

				if diff := cmp.Diff(syscallExecArgsColl[0].argv[3], "/path/to/launcher"); diff != "" {
					t.Fatalf(`syscall.Exec Argv did not match: (-got +want)\n%s`, diff)
				}
				if diff := cmp.Diff(syscallExecArgsColl[0].argv[4], "some-web-process"); diff != "" {
					t.Fatalf(`syscall.Exec Argv did not match: (-got +want)\n%s`, diff)
				}
			})

			when("default start process type is not in the process types", func() {
				it("should return an error", func() {
					launcher.DefaultProcessType = "not-exist"

					err := launcher.Launch("/path/to/launcher", "")
					if err == nil {
						t.Fatalf("expected launch to return an error")
					}

					if len(syscallExecArgsColl) != 0 {
						t.Fatalf("expected syscall.Exec to not be called: actual %v", syscallExecArgsColl)
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
						t.Fatalf("expected syscall.Exec to be called once: actual %v", syscallExecArgsColl)
					}

					if diff := cmp.Diff(syscallExecArgsColl[0].argv[4], "some-worker-process"); diff != "" {
						t.Fatalf(`syscall.Exec Argv did not match: (-got +want)\n%s`, diff)
					}
				})
			})

			when("start command does NOT match a process type", func() {
				it("should run the start command", func() {
					if err := launcher.Launch("/path/to/launcher", "some-different-process"); err != nil {
						t.Fatal(err)
					}

					if len(syscallExecArgsColl) != 1 {
						t.Fatalf("expected syscall.Exec to be called once: actual %v", syscallExecArgsColl)
					}

					if diff := cmp.Diff(syscallExecArgsColl[0].argv[4], "some-different-process"); diff != "" {
						t.Fatalf(`syscall.Exec Argv did not match: (-got +want)\n%s`, diff)
					}
				})
			})
		})

		when("buildpacks provided profile.d scripts", func() {
			it.Before(func() {
				if err := ioutil.WriteFile(filepath.Join(tmpDir, "launch", "app", "start"), []byte("#!/usr/bin/env bash\necho hi from app\n"), 0755); err != nil {
					t.Fatal(err)
				}
				launcher.Processes = []lifecycle.Process{
					{Type: "start", Command: "./start"},
				}
				launcher.Buildpacks = []string{"bp.1", "bp.2"}
				launcher.Exec = syscallExecWithStdout(t, tmpDir)

				if err := os.MkdirAll(filepath.Join(tmpDir, "launch", "bp.1", "layer", "profile.d"), 0755); err != nil {
					t.Fatal(err)
				}
				if err := ioutil.WriteFile(filepath.Join(tmpDir, "launch", "bp.1", "layer", "profile.d", "apple"), []byte("echo apple"), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(tmpDir, "launch", "bp.2", "layer", "profile.d"), 0755); err != nil {
					t.Fatal(err)
				}
				if err := ioutil.WriteFile(filepath.Join(tmpDir, "launch", "bp.2", "layer", "profile.d", "banana"), []byte("echo banana"), 0644); err != nil {
					t.Fatal(err)
				}
			})

			it("should run them in buildpack order", func() {
				if err := launcher.Launch("/path/to/launcher", "start"); err != nil {
					t.Fatal(err)
				}

				stdout, err := ioutil.ReadFile(filepath.Join(tmpDir, "stdout"))
				if err != nil {
					t.Fatal(err)
				}
				expected := "apple\nbanana\nhi from app\n"

				if len(stdout) == 0 {
					stderr, err := ioutil.ReadFile(filepath.Join(tmpDir, "stderr"))
					if err != nil {
						t.Fatal(err)
					}
					t.Fatalf("stdout was empty: stderr: %s", stderr)
				}
				if diff := cmp.Diff(string(stdout), expected); diff != "" {
					t.Fatalf(`syscall.Exec stdout did not match: (-got +want)\n%s`, diff)
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

					stdout, err := ioutil.ReadFile(filepath.Join(tmpDir, "stdout"))
					if err != nil {
						t.Fatal(err)
					}
					expected := "banana\napple\nhi from app\n"

					if len(stdout) == 0 {
						stderr, _ := ioutil.ReadFile(filepath.Join(tmpDir, "stderr"))
						t.Fatalf("stdout was empty: stderr: %s", stderr)
					}
					if diff := cmp.Diff(string(stdout), expected); diff != "" {
						t.Fatalf(`syscall.Exec stdout did not match: (-got +want)\n%s`, diff)
					}
				})
			})

			when("app has '.profile'", func() {
				it.Before(func() {
					if err := ioutil.WriteFile(filepath.Join(tmpDir, "launch", "app", ".profile"), []byte("echo from profile"), 0644); err != nil {
						t.Fatal(err)
					}
				})

				it("should source .profile", func() {
					if err := launcher.Launch("/path/to/launcher", "start"); err != nil {
						t.Fatal(err)
					}

					stdout, err := ioutil.ReadFile(filepath.Join(tmpDir, "stdout"))
					if err != nil {
						t.Fatal(err)
					}
					expected := "apple\nbanana\nfrom profile\nhi from app\n"

					if len(stdout) == 0 {
						stderr, err := ioutil.ReadFile(filepath.Join(tmpDir, "stderr"))
						if err != nil {
							t.Fatal(err)
						}
						t.Fatalf("stdout was empty: stderr: %s", stderr)
					}
					if diff := cmp.Diff(string(stdout), expected); diff != "" {
						t.Fatalf(`syscall.Exec stdout did not match: (-got +want)\n%s`, diff)
					}
				})
			})
		})
	})
}

func syscallExecWithStdout(t *testing.T, tmpDir string) func(argv0 string, argv []string, envv []string) error {
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
