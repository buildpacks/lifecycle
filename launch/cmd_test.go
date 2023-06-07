package launch_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/launch"
	hl "github.com/buildpacks/lifecycle/launch/testhelpers"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestCmd(t *testing.T) {
	spec.Run(t, "Cmd", testCmd, spec.Report(report.Terminal{}))
}

func testCmd(t *testing.T, when spec.G, it spec.S) {
	var (
		shell      launch.Shell
		tmpDir     string
		defaultDir string
		err        error
	)

	it.Before(func() {
		defaultDir, err = os.Getwd()
		h.AssertNil(t, err)
		h.SkipIf(t, runtime.GOOS != "windows", "skip cmd tests on unix")
		tmpDir, err = os.MkdirTemp("", "shell-test")
		h.AssertNil(t, err)
		shell = &launch.CmdShell{Exec: hl.SyscallExecWithStdout(t, tmpDir)}
	})

	it.After(func() {
		h.AssertNil(t, os.RemoveAll(tmpDir))
	})

	when("#Launch", func() {
		var process launch.ShellProcess

		when("is not script", func() {
			when("there are profiles", func() {
				it.Before(func() {
					h.AssertNil(t, err)
					process = launch.ShellProcess{
						Script:  false,
						Command: "echo",
						Args:    []string{"profile env: '!PROFILE_VAR!'"},
						Env: []string{
							"SOME_VAR=some-val",
						},
						WorkingDirectory: defaultDir,
					}
					process.Profiles = []string{
						filepath.Join("testdata", "profiles", "print_argv0.bat"),
						filepath.Join("testdata", "profiles", "print_env.bat"),
						filepath.Join("testdata", "profiles", "set_env.bat"),
					}
				})

				it("runs the profiles from the default directory", func() {
					process.Profiles = []string{
						filepath.Join("testdata", "profiles", "pwd.bat"),
					}
					err = shell.Launch(process)
					h.AssertNil(t, err)
					stdout := rdfile(t, filepath.Join(tmpDir, "stdout"))
					h.AssertStringContains(t, stdout, fmt.Sprintf("profile directory: %s\r\n", defaultDir))
				})

				it("runs the command from the working directory", func() {
					process.WorkingDirectory = tmpDir
					process.Command = "echo"
					process.Args = []string{
						"process",
						"working",
						"directory:",
						"&&",
						"cd",
					}
					err = shell.Launch(process)
					h.AssertNil(t, err)
					stdout := rdfile(t, filepath.Join(tmpDir, "stdout"))
					h.AssertStringContains(t, stdout, fmt.Sprintf("process working directory: \r\n%s\r\n", tmpDir))
				})

				it("sets argv0 for profile scripts to profile script path", func() {
					err := shell.Launch(process)
					h.AssertNil(t, err)
					stdout := rdfile(t, filepath.Join(tmpDir, "stdout"))
					if len(stdout) == 0 {
						stderr := rdfile(t, filepath.Join(tmpDir, "stderr"))
						t.Fatalf("stdout was empty: stderr: %s\n", stderr)
					}
					h.AssertStringContains(t, stdout, fmt.Sprintf("profile argv0: '%s'", filepath.Join("testdata", "profiles", "print_argv0.bat")))
				})

				it("sets env for profile scripts", func() {
					err := shell.Launch(process)
					h.AssertNil(t, err)
					stdout := rdfile(t, filepath.Join(tmpDir, "stdout"))
					if len(stdout) == 0 {
						stderr := rdfile(t, filepath.Join(tmpDir, "stderr"))
						t.Fatalf("stdout was empty: stderr: %s\n", stderr)
					}
					h.AssertStringContains(t, stdout, "SOME_VAR: 'some-val'")
				})

				it("env vars set in profile scripts are available to the command", func() {
					err := shell.Launch(process)
					h.AssertNil(t, err)
					stdout := rdfile(t, filepath.Join(tmpDir, "stdout"))
					if len(stdout) == 0 {
						stderr := rdfile(t, filepath.Join(tmpDir, "stderr"))
						t.Fatalf("stdout was empty: stderr: %s\n", stderr)
					}
					h.AssertStringContains(t, stdout, "profile env: 'some-profile-var'")
				})
			})

			it("sets env", func() {
				process = launch.ShellProcess{
					Script:  false,
					Command: `echo`,
					Args:    []string{"SOME_VAR: '%SOME_VAR%'"},
					Env: []string{
						"SOME_VAR=some-val",
					},
					WorkingDirectory: defaultDir,
				}
				err := shell.Launch(process)
				h.AssertNil(t, err)
				stdout := rdfile(t, filepath.Join(tmpDir, "stdout"))
				if len(stdout) == 0 {
					stderr := rdfile(t, filepath.Join(tmpDir, "stderr"))
					t.Fatalf("stdout was empty: stderr: %s\n", stderr)
				}
				h.AssertStringContains(t, stdout, "SOME_VAR: 'some-val'")
			})
		})
	})
}
