// +build linux darwin

package testhelpers

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/buildpacks/lifecycle/launch"
)

func SyscallExecWithStdout(t *testing.T, tmpDir string) launch.ExecFunc {
	t.Helper()
	fstdin, err := os.Create(filepath.Join(tmpDir, "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	fstdout, err := os.Create(filepath.Join(tmpDir, "stdout"))
	if err != nil {
		_ = fstdin.Close()
		t.Fatal(err)
	}
	fstderr, err := os.Create(filepath.Join(tmpDir, "stderr"))
	if err != nil {
		_ = fstdin.Close()
		_ = fstdout.Close()
		t.Fatal(err)
	}

	return func(argv0 string, argv []string, envv []string) error {
		defer fstdin.Close()
		defer fstdout.Close()
		defer fstderr.Close()
		pid, err := syscall.ForkExec(argv0, argv, &syscall.ProcAttr{
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
