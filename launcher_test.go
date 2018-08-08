package lifecycle_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
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
			DefaultAppDir:      filepath.Join(tmpDir, "launch", "app"),
			Processes: []lifecycle.Process{
				{Type: "other", Command: "some-other-process"},
				{Type: "web", Command: "some-web-process"},
				{Type: "worker", Command: "some-worker-process"},
			},
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
			it("runs the default process type", func() {
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
				if diff := cmp.Diff(syscallExecArgsColl[0].argv[5], "some-web-process"); diff != "" {
					t.Fatalf(`syscall.Exec Argv did not match: (-got +want)\n%s`, diff)
				}
			})

			when("default start process type is not in the process types", func() {
				it("returns an error", func() {
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
				it("runs that process type", func() {
					if err := launcher.Launch("/path/to/launcher", "worker"); err != nil {
						t.Fatal(err)
					}

					if len(syscallExecArgsColl) != 1 {
						t.Fatalf("expected syscall.Exec to be called once: actual %v", syscallExecArgsColl)
					}

					if diff := cmp.Diff(syscallExecArgsColl[0].argv[5], "some-worker-process"); diff != "" {
						t.Fatalf(`syscall.Exec Argv did not match: (-got +want)\n%s`, diff)
					}
				})
			})
			when("start command does NOT match a process type", func() {
				it("runs the start command", func() {
					if err := launcher.Launch("/path/to/launcher", "some-different-process"); err != nil {
						t.Fatal(err)
					}

					if len(syscallExecArgsColl) != 1 {
						t.Fatalf("expected syscall.Exec to be called once: actual %v", syscallExecArgsColl)
					}

					if diff := cmp.Diff(syscallExecArgsColl[0].argv[5], "some-different-process"); diff != "" {
						t.Fatalf(`syscall.Exec Argv did not match: (-got +want)\n%s`, diff)
					}
				})
			})
		})
	})
}
