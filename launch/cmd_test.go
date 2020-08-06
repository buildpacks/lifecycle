package launch_test

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/lifecycle/launch"
	hl "github.com/buildpacks/lifecycle/launch/testhelpers"
	h "github.com/buildpacks/lifecycle/testhelpers"
)

func TestCmd(t *testing.T) {
	rand.Seed(time.Now().UTC().UnixNano())
	spec.Run(t, "Cmd", testCmd, spec.Report(report.Terminal{}))
}

func testCmd(t *testing.T, when spec.G, it spec.S) {
	var (
		shell  launch.Shell
		tmpDir string
	)

	it.Before(func() {
		h.SkipIf(t, runtime.GOOS != "windows", "skip cmd tests on unix")
		var err error
		tmpDir, err = ioutil.TempDir("", "shell-test")
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
					process = launch.ShellProcess{
						Script:  false,
						Command: "echo",
						Args:    []string{"profile env: '%PROFILE_VAR%'"},
						Env: []string{
							"SOME_VAR=some-val",
						},
					}
					process.Profiles = []string{
						filepath.Join("testdata", "profiles", "print_argv0.bat"),
						filepath.Join("testdata", "profiles", "print_env.bat"),
						filepath.Join("testdata", "profiles", "set_env.bat"),
					}
				})

				it("sets argv0 for profile scripts to profile script path", func() {
					err := shell.Launch(process)
					h.AssertNil(t, err)
					stdout := rdfile(t, filepath.Join(tmpDir, "stdout"))
					if len(stdout) == 0 {
						stderr := rdfile(t, filepath.Join(tmpDir, "stderr"))
						t.Fatalf("stdout was empty: stderr: %s\n", stderr)
					}
					h.AssertStringContains(t, stdout, fmt.Sprintf("profile arv0: '%s'", filepath.Join("testdata", "profiles", "print_argv0.bat")))
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

				it.Pend("env vars set in profile scripts are available to the command", func() {
					// currently broken

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
