// +build windows

package testsyscall

import "testing"

func SyscallExecWithStdout(t *testing.T, tmpDir string) func(argv0 string, argv []string, envv []string) error {
	//panic("Not implemented on Windows")
	return nil
}
