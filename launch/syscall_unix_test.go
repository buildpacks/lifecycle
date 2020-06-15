// +build linux darwin

package launch_test

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func SyscallExecWithStdout(t *testing.T, tmpDir string) func(argv0 string, argv []string, envv []string) error {
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
