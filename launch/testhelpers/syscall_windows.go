// +build windows

package testhelpers

import (
	"os"
	"os/exec"
	"path/filepath"
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
		c := exec.Command(argv[0], argv[1:]...)
		c.Env = envv
		c.Stdin = fstdin
		c.Stdout = fstdout
		c.Stderr = fstderr
		return c.Run()
	}
}
