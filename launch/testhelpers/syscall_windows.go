// +build windows

package testhelpers

import (
	"os"
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
		t.Fatal(err)
	}
	fstderr, err := os.Create(filepath.Join(tmpDir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}

	return func(argv0 string, argv []string, envv []string) error {
		c := launch.OSExecCmd(argv, envv)
		c.Stdin = fstdin
		c.Stdout = fstdout
		c.Stderr = fstderr
		return c.Run()
	}
}
